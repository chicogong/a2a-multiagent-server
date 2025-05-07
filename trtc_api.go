// TRTC API client implementation for AI conversation management
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	trtc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/trtc/v20190722"
)

var (
	trtcClient     *trtc.Client
	trtcClientOnce sync.Once
)

// Voice constants for common voice types
const (
	VoiceTypeXiaoMei  = 601005
	VoiceTypeXiaoShuai = 601008
)

// getTRTCClient returns a singleton TRTC client
func getTRTCClient() *trtc.Client {
	trtcClientOnce.Do(func() {
		secretID := os.Getenv("TRTC_SECRET_ID")
		secretKey := os.Getenv("TRTC_SECRET_KEY")
		region := os.Getenv("TRTC_REGION")
		endpoint := os.Getenv("TRTC_ENDPOINT")
		
		if secretID == "" || secretKey == "" {
			log.Printf("Warning: TRTC credentials not found in environment variables")
		}
		
		credential := common.NewCredential(secretID, secretKey)
		cpf := profile.NewClientProfile()
		cpf.HttpProfile.Endpoint = endpoint
		
		var err error
		trtcClient, err = trtc.NewClient(credential, region, cpf)
		if err != nil {
			panic(fmt.Sprintf("Failed to create TRTC client: %v", err))
		}
	})
	return trtcClient
}

// UpdateAIConversation updates the AI conversation configuration
func UpdateAIConversation(taskID, ttsConfig string) error {
	request := trtc.NewUpdateAIConversationRequest()
	request.TaskId = common.StringPtr(taskID)
	request.TTSConfig = common.StringPtr(ttsConfig)

	_, err := getTRTCClient().UpdateAIConversation(request)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return fmt.Errorf("API error: %s", sdkErr)
		}
		return fmt.Errorf("update failed: %w", err)
	}
	
	return nil
}

// UpdateAIConversationXiaoMei updates the AI conversation with XiaoMei's voice
func UpdateAIConversationXiaoMei(taskID string) error {
	appID, _ := strconv.Atoi(os.Getenv("TTS_APP_ID"))
	secretID := os.Getenv("TTS_SECRET_ID")
	secretKey := os.Getenv("TTS_SECRET_KEY")
	
	ttsConfig := fmt.Sprintf(`{
		"TTSType": "tencent",
		"AppId": %d,
		"SecretId": "%s",
		"SecretKey": "%s",
		"VoiceType": %d,
		"Speed": 1
	}`, appID, secretID, secretKey, VoiceTypeXiaoMei)
	
	return UpdateAIConversation(taskID, ttsConfig)
}

// UpdateAIConversationXiaoShuai updates the AI conversation with XiaoShuai's voice
func UpdateAIConversationXiaoShuai(taskID string) error {
	appID, _ := strconv.Atoi(os.Getenv("TTS_APP_ID"))
	secretID := os.Getenv("TTS_SECRET_ID")
	secretKey := os.Getenv("TTS_SECRET_KEY")
	
	ttsConfig := fmt.Sprintf(`{
		"TTSType": "tencent",
		"AppId": %d,
		"SecretId": "%s",
		"SecretKey": "%s",
		"VoiceType": %d,
		"Speed": 1
	}`, appID, secretID, secretKey, VoiceTypeXiaoShuai)
	
	return UpdateAIConversation(taskID, ttsConfig)
}

// ControlAIConversation sends control commands to an AI conversation
func ControlAIConversation(taskID, text string) error {
	request := trtc.NewControlAIConversationRequest()
	request.TaskId = common.StringPtr(taskID)
	request.Command = common.StringPtr("ServerPushText")
	request.ServerPushText = &trtc.ServerPushText{
		Text: common.StringPtr(text),
	}

	_, err := getTRTCClient().ControlAIConversation(request)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return fmt.Errorf("API error: %s", sdkErr)
		}
		return fmt.Errorf("control failed: %w", err)
	}
	
	return nil
} 
