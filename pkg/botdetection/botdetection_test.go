package botdetection

import "testing"

func TestIsBotUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		wantBot   bool
	}{
		// Human browsers
		{
			name:      "Chrome browser",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantBot:   false,
		},
		{
			name:      "Firefox browser",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0",
			wantBot:   false,
		},
		{
			name:      "Safari browser",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Safari/605.1.15",
			wantBot:   false,
		},
		// Bots and scanners
		{
			name:      "Empty user agent",
			userAgent: "",
			wantBot:   true,
		},
		{
			name:      "Generic bot",
			userAgent: "Mozilla/5.0 (compatible; bot/1.0)",
			wantBot:   true,
		},
		{
			name:      "Googlebot",
			userAgent: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			wantBot:   true,
		},
		{
			name:      "HeadlessChrome",
			userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/120.0.0.0 Safari/537.36",
			wantBot:   true,
		},
		{
			name:      "SafeLinks scanner",
			userAgent: "Microsoft Outlook SafeLinks PreFetch",
			wantBot:   true,
		},
		{
			name:      "Proofpoint",
			userAgent: "Proofpoint URL Defense",
			wantBot:   true,
		},
		{
			name:      "Mimecast",
			userAgent: "Mimecast URL Protection",
			wantBot:   true,
		},
		{
			name:      "Python requests",
			userAgent: "python-requests/2.28.1",
			wantBot:   true,
		},
		{
			name:      "cURL",
			userAgent: "curl/7.68.0",
			wantBot:   true,
		},
		{
			name:      "wget",
			userAgent: "Wget/1.20.3 (linux-gnu)",
			wantBot:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBotUserAgent(tt.userAgent)
			if got != tt.wantBot {
				t.Errorf("IsBotUserAgent() = %v, want %v", got, tt.wantBot)
			}
		})
	}
}
