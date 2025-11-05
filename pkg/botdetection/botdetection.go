package botdetection

import "strings"

// IsBotUserAgent checks if the given user agent string appears to be from a bot or automated scanner
func IsBotUserAgent(userAgent string) bool {
	// Empty user-agent is suspicious
	if userAgent == "" {
		return true
	}

	// Convert to lowercase for case-insensitive matching
	ua := strings.ToLower(userAgent)

	// Known bot/crawler/scanner patterns
	botPatterns := []string{
		"bot",
		"crawler",
		"spider",
		"scanner",
		"linkcheck",
		"security",
		"headlesschrome",
		"phantomjs",
		"selenium",
		"safelinks",       // Microsoft SafeLinks
		"proofpoint",      // Proofpoint email security
		"mimecast",        // Mimecast email security
		"atp",             // Advanced Threat Protection
		"barracuda",       // Barracuda email security
		"forcepoint",      // Forcepoint email security
		"cisco ironport",  // Cisco email security
		"symantec",        // Symantec email security
		"mcafee",          // McAfee email security
		"trend micro",     // Trend Micro email security
		"sophos",          // Sophos email security
		"fireeye",         // FireEye email security
		"emailsecurity",   // Generic email security
		"urldefense",      // URL defense systems
		"linkprotect",     // Link protection systems
		"urlscan",         // URL scanning
		"urlfilter",       // URL filtering
		"emailprotection", // Email protection systems
		"antivirus",       // Antivirus scanners
		"malware",         // Malware scanners
		"threatdetection", // Threat detection systems
		"securityscanner", // Security scanners
		"python-requests", // Python requests library (often used by bots)
		"curl",            // cURL (command line tool)
		"wget",            // wget (command line tool)
		"java",            // Java HTTP clients (often automated)
		"go-http-client",  // Go HTTP client (often automated)
		"postman",         // API testing tool
	}

	for _, pattern := range botPatterns {
		if strings.Contains(ua, pattern) {
			return true
		}
	}

	return false
}
