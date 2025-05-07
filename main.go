// Tencent is pleased to support the open source community by making a2a-go available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.  All rights reserved.
//
// a2a-go is licensed under the Apache License Version 2.0.

// Package main implements a streaming server for the A2A protocol.
package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"trpc.group/trpc-go/trpc-a2a-go/server"

	"github.com/sashabaranov/go-openai"
	"trpc.group/trpc-go/trpc-a2a-go/protocol"
	"trpc.group/trpc-go/trpc-a2a-go/taskmanager"
)

// streamingTaskProcessor implements the TaskProcessor interface for streaming responses.
type streamingTaskProcessor struct {
	openaiClient *openai.Client
	openaiModel  string
}

// Process implements the core streaming logic.
func (p *streamingTaskProcessor) Process(
		ctx context.Context,
		taskID string,
		message protocol.Message,
		handle taskmanager.TaskHandle,
) error {
	log.Printf("Processing streaming task %s...", taskID)
	log.Printf("Task %s received message: %s", taskID, message)

	text := extractText(message)
	if text == "" {
		errMsg := "input message must contain text"
		log.Printf("Task %s failed: %s", taskID, errMsg)

		failedMessage := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart(errMsg)},
		)
		_ = handle.UpdateStatus(protocol.TaskStateFailed, &failedMessage)
		return fmt.Errorf(errMsg)
	}

	isStreaming := handle.IsStreamingRequest()

	if !isStreaming {
		log.Printf("Task %s using non-streaming mode", taskID)
		return p.processNonStreaming(ctx, taskID, text, handle)
	}

	log.Printf("Task %s using streaming mode", taskID)

	initialMessage := protocol.NewMessage(
		protocol.MessageRoleAgent,
		[]protocol.Part{protocol.NewTextPart("Starting to process your streaming data with OpenAI...")},
	)
	if err := handle.UpdateStatus(protocol.TaskStateWorking, &initialMessage); err != nil {
		log.Printf("Error updating initial status for task %s: %v", taskID, err)
		return err
	}

	if err := p.processWithOpenAIStreaming(ctx, taskID, text, handle); err != nil {
		log.Printf("Error processing with OpenAI for task %s: %v", taskID, err)
		failedMessage := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart(fmt.Sprintf("Failed to process with OpenAI: %v", err))},
		)
		_ = handle.UpdateStatus(protocol.TaskStateFailed, &failedMessage)
		return err
	}

	log.Printf("Task %s streaming completed successfully.", taskID)
	return nil
}

// processWithOpenAIStreaming sends the text to OpenAI API with streaming enabled
// and processes the streaming response
func (p *streamingTaskProcessor) processWithOpenAIStreaming(
		ctx context.Context,
		taskID string,
		text string,
		handle taskmanager.TaskHandle,
) error {
	intent, err := p.detectIntent(ctx, text, taskID)
	if err != nil {
		log.Printf("Task %s intent detection failed: %v", taskID, err)
		return fmt.Errorf("intent detection failed: %w", err)
	}

	log.Printf("Task %s will be processed by %s", taskID, intent)

	req := openai.ChatCompletionRequest{
		Model: p.openaiModel,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: p.getAssistantPrompt(intent),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: text,
			},
		},
		Stream: true,
	}

	stream, err := p.openaiClient.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create OpenAI streaming request: %w", err)
	}
	defer stream.Close()

	chunkIndex := 0
	var fullResponse strings.Builder
	startTime := time.Now()
	firstTokenReceived := false

	for {
		if err := ctx.Err(); err != nil {
			log.Printf("Task %s canceled during OpenAI streaming: %v", taskID, err)
			_ = handle.UpdateStatus(protocol.TaskStateCanceled, nil)
			return err
		}

		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to receive OpenAI streaming response: %w", err)
		}

		content := response.Choices[0].Delta.Content
		if content == "" {
			continue
		}

		if !firstTokenReceived {
			elapsed := time.Since(startTime)
			log.Printf("Task %s: Time to first token: %v", taskID, elapsed)
			firstTokenReceived = true
		}

		fullResponse.WriteString(content)

		log.Printf("Task %s: Sending chunk %d, content length: %d",
			taskID, chunkIndex+1, len(content))

		statusMsg := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart(content)},
		)

		if err := handle.UpdateStatus(protocol.TaskStateWorking, &statusMsg); err != nil {
			log.Printf("Error updating progress status for task %s: %v", taskID, err)
		}

		chunkArtifact := protocol.Artifact{
			Name:        stringPtr(fmt.Sprintf("Chunk %d", chunkIndex+1)),
			Description: stringPtr("Streaming chunk from OpenAI"),
			Index:       chunkIndex,
			Parts:       []protocol.Part{protocol.NewTextPart(content)},
			Append:      boolPtr(chunkIndex > 0),
			Metadata: map[string]interface{}{
				"timestamp":    time.Now().UnixNano(),
				"chunk_size":   len(content),
				"chunk_index":  chunkIndex,
				"total_length": fullResponse.Len(),
				"model":        p.openaiModel,
				"is_streaming": true,
			},
		}

		if err := handle.AddArtifact(chunkArtifact); err != nil {
			log.Printf("Error adding artifact for chunk %d of task %s: %v", chunkIndex+1, taskID, err)
		}

		chunkIndex++
	}

	if chunkIndex > 0 {
		lastChunkArtifact := protocol.Artifact{
			Name:        stringPtr(fmt.Sprintf("Chunk %d", chunkIndex)),
			Description: stringPtr("Final chunk from OpenAI"),
			Index:       chunkIndex - 1,
			Parts:       []protocol.Part{},
			LastChunk:   boolPtr(true),
			Metadata: map[string]interface{}{
				"timestamp":     time.Now().UnixNano(),
				"total_chunks":  chunkIndex,
				"total_length":  fullResponse.Len(),
				"model":         p.openaiModel,
				"is_streaming":  true,
				"is_last_chunk": true,
			},
		}
		if err := handle.AddArtifact(lastChunkArtifact); err != nil {
			log.Printf("Error adding final chunk marker for task %s: %v", taskID, err)
		}
	}

	completeMessage := protocol.NewMessage(
		protocol.MessageRoleAgent,
		[]protocol.Part{
			protocol.NewTextPart(
				fmt.Sprintf("Processing complete. Received %d chunks.", chunkIndex))},
	)
	if err := handle.UpdateStatus(protocol.TaskStateCompleted, &completeMessage); err != nil {
		log.Printf("Error updating final status for task %s: %v", taskID, err)
		return fmt.Errorf("failed to update final task status: %w", err)
	}

	return nil
}

// processWithOpenAINonStreaming sends the text to OpenAI API without streaming
// and returns the complete response
func (p *streamingTaskProcessor) processWithOpenAINonStreaming(
		ctx context.Context,
		text string,
		taskID string,
) (string, error) {
	intent, err := p.detectIntent(ctx, text, taskID)
	if err != nil {
		return "", fmt.Errorf("intent detection failed: %w", err)
	}

	req := openai.ChatCompletionRequest{
		Model: p.openaiModel,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: p.getAssistantPrompt(intent),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: text,
			},
		},
	}

	resp, err := p.openaiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create OpenAI request: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in OpenAI response")
	}

	return resp.Choices[0].Message.Content, nil
}

// processNonStreaming handles processing for non-streaming requests
func (p *streamingTaskProcessor) processNonStreaming(
		ctx context.Context,
		taskID string,
		text string,
		handle taskmanager.TaskHandle,
) error {
	initialMessage := protocol.NewMessage(
		protocol.MessageRoleAgent,
		[]protocol.Part{protocol.NewTextPart("Processing your text with OpenAI...")},
	)
	if err := handle.UpdateStatus(protocol.TaskStateWorking, &initialMessage); err != nil {
		log.Printf("Error updating initial status for task %s: %v", taskID, err)
		return err
	}

	processedText, err := p.processWithOpenAINonStreaming(ctx, text, taskID)
	if err != nil {
		log.Printf("Error processing with OpenAI for task %s: %v", taskID, err)
		failedMessage := protocol.NewMessage(
			protocol.MessageRoleAgent,
			[]protocol.Part{protocol.NewTextPart(fmt.Sprintf("Failed to process with OpenAI: %v", err))},
		)
		_ = handle.UpdateStatus(protocol.TaskStateFailed, &failedMessage)
		return err
	}

	artifact := protocol.Artifact{
		Name:        stringPtr("Processed Text"),
		Description: stringPtr("Complete processed text from OpenAI"),
		Index:       0,
		Parts:       []protocol.Part{protocol.NewTextPart(processedText)},
		LastChunk:   boolPtr(true),
		Metadata: map[string]interface{}{
			"timestamp":    time.Now().UnixNano(),
			"total_length": len(processedText),
			"model":        p.openaiModel,
			"is_streaming": false,
		},
	}

	if err := handle.AddArtifact(artifact); err != nil {
		log.Printf("Error adding artifact for task %s: %v", taskID, err)
	}

	completeMessage := protocol.NewMessage(
		protocol.MessageRoleAgent,
		[]protocol.Part{
			protocol.NewTextPart(
				fmt.Sprintf("Processing complete. OpenAI response received."))},
	)
	if err := handle.UpdateStatus(protocol.TaskStateCompleted, &completeMessage); err != nil {
		log.Printf("Error updating final status for task %s: %v", taskID, err)
		return fmt.Errorf("failed to update final task status: %w", err)
	}

	log.Printf("Task %s non-streaming completed successfully.", taskID)
	return nil
}

// extractText extracts the first text part from a message.
func extractText(message protocol.Message) string {
	for _, part := range message.Parts {
		if p, ok := part.(protocol.TextPart); ok {
			return p.Text
		}
	}
	return ""
}

// Helper functions for environment variables
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// Helper functions to create pointers
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

// detectIntent determines which AI assistant the user wants to talk to
func (p *streamingTaskProcessor) detectIntent(ctx context.Context, text, taskID string) (string, error) {
	req := openai.ChatCompletionRequest{
		Model: p.openaiModel,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleSystem,
				Content: `You are an intent detection assistant. You need to determine which AI assistant the user wants to talk to.
Options are:
1. XiaoMei(小美): Female assistant, lively and cute personality, can solve female-related issues.
2. XiaoShuai(小帅): Male assistant, sunny and cheerful personality, can solve male-related issues.
Please only reply with "XiaoMei" or "XiaoShuai"`,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: text,
			},
		},
	}

	resp, err := p.openaiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("intent detection failed: %w", err)
	}

	intent := strings.TrimSpace(resp.Choices[0].Message.Content)
	if intent != "XiaoMei" && intent != "XiaoShuai" {
		log.Printf("Could not clearly identify intent, defaulting to XiaoMei")
		intent = "XiaoMei"
	} else {
		log.Printf("Intent detection result: User wants to talk to %s", intent)
	}

	// Call TRTC API to update TTS voice based on detected intent
	if intent == "XiaoMei" {
		log.Printf("Starting TTS update for XiaoMei, taskid: %s", taskID)
		if len(taskID) > 64 {
			if err := UpdateAIConversationXiaoMei(taskID); err != nil {
				log.Printf("Failed to update TTS for XiaoMei: %v", err)
			} else {
				log.Printf("Successfully updated TTS for XiaoMei")
			}
		} else {
			log.Printf("Invalid taskID length for XiaoMei: %s", taskID)
		}
	} else {
		log.Printf("Starting TTS update for XiaoShuai, taskid: %s", taskID)
		if len(taskID) > 64 {
			if err := UpdateAIConversationXiaoShuai(taskID); err != nil {
				log.Printf("Failed to update TTS for XiaoShuai: %v", err)
			} else {
				log.Printf("Successfully updated TTS for XiaoShuai")
			}
		} else {
			log.Printf("Invalid taskID length for XiaoShuai: %s", taskID)
		}
	}

	return intent, nil
}

// getAssistantPrompt returns the system prompt for the specified assistant
func (p *streamingTaskProcessor) getAssistantPrompt(intent string) string {
	prompts := map[string]string{
		"XiaoMei":   "You are an AI assistant named XiaoMei(小美). Keep the conversation casual, lively, and concise",
		"XiaoShuai": "You are an AI assistant named XiaoShuai(小帅). Keep the conversation casual, humorous, and concise",
	}
	return prompts[intent]
}

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Get configuration from environment variables
	host := getEnvOrDefault("SERVER_HOST", "localhost")
	port := getEnvIntOrDefault("SERVER_PORT", 8080)
	openaiModel := getEnvOrDefault("OPENAI_MODEL", "gpt-3.5-turbo")
	baseURL := getEnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1")
	openaiKey := os.Getenv("OPENAI_API_KEY")

	if openaiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	address := fmt.Sprintf("%s:%d", host, port)
	serverURL := fmt.Sprintf("http://%s/", address)

	config := openai.DefaultConfig(openaiKey)
	config.BaseURL = baseURL
	openaiClient := openai.NewClientWithConfig(config)

	description := "A2A streaming example server that processes text using OpenAI API"
	agentCard := server.AgentCard{
		Name:        "OpenAI Text Processor",
		Description: &description,
		URL:         serverURL,
		Version:     "1.0.0",
		Provider: &server.AgentProvider{
			Name: "A2A-Go Examples",
		},
		Capabilities: server.AgentCapabilities{
			Streaming:              true,
			StateTransitionHistory: true,
		},
		DefaultInputModes:  []string{string(protocol.PartTypeText)},
		DefaultOutputModes: []string{string(protocol.PartTypeText)},
		Skills: []server.AgentSkill{
			{
				ID:          "openai_processor",
				Name:        "OpenAI Text Processor",
				Description: stringPtr("Input: Any text\nOutput: OpenAI API response delivered incrementally\n\nThis agent sends your text to OpenAI API and streams back the response."),
				Tags:        []string{"text", "stream", "openai", "example"},
				Examples: []string{
					"Explain quantum computing in simple terms",
					"Write a short poem about artificial intelligence",
					"What are the main features of Go programming language?",
				},
				InputModes:  []string{string(protocol.PartTypeText)},
				OutputModes: []string{string(protocol.PartTypeText)},
			},
		},
	}

	processor := &streamingTaskProcessor{
		openaiClient: openaiClient,
		openaiModel:  openaiModel,
	}

	taskManager, err := taskmanager.NewMemoryTaskManager(processor)
	if err != nil {
		log.Fatalf("Failed to create task manager: %v", err)
	}

	srv, err := server.NewA2AServer(agentCard, taskManager)
	if err != nil {
		log.Fatalf("Failed to create A2A server: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting streaming server on %s...", address)
		if err := srv.Start(address); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sig := <-sigChan
	log.Printf("Received signal %v, shutting down server...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		log.Fatalf("Error during server shutdown: %v", err)
	}

	log.Println("Server shutdown complete")
}
