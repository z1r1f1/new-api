package chatgptimg

import "github.com/QuantumNous/new-api/service"

func firstChatGPTWebTiming(timings ...*service.ChatGPTWebTiming) *service.ChatGPTWebTiming {
	for _, timing := range timings {
		if timing != nil {
			return timing
		}
	}
	return nil
}
