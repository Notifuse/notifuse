package templates

import (
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// CreateSupabaseReauthenticationEmailStructure creates the detailed MJML structure for the reauthentication email
func CreateSupabaseReauthenticationEmailStructure() (notifuse_mjml.EmailBlock, error) {
	jsonTemplate := `{
  "emailTree": {
    "id": "mjml-1",
    "type": "mjml",
    "attributes": {},
    "children": [
      {
        "id": "head-1",
        "type": "mj-head",
        "attributes": {},
        "children": []
      },
      {
        "id": "body-1",
        "type": "mj-body",
        "attributes": {
          "width": "600px",
          "backgroundColor": "#ffffff"
        },
        "children": [
          {
            "id": "wrapper-1",
            "type": "mj-wrapper",
            "attributes": {
              "paddingTop": "20px",
              "paddingRight": "20px",
              "paddingBottom": "20px",
              "paddingLeft": "20px"
            },
            "children": [
              {
                "id": "section-1",
                "type": "mj-section",
                "attributes": {
                  "backgroundColor": "transparent",
                  "paddingTop": "20px",
                  "paddingRight": "0px",
                  "paddingBottom": "20px",
                  "paddingLeft": "0px",
                  "textAlign": "center"
                },
                "children": [
                  {
                    "id": "column-1",
                    "type": "mj-column",
                    "attributes": {
                      "width": "100%"
                    },
                    "children": [
                      {
                        "id": "image-1",
                        "type": "mj-image",
                        "attributes": {
                          "align": "center",
                          "src": "https://storage.googleapis.com/readonlydemo/logo-large.png",
                          "width": "120px",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px"
                        }
                      },
                      {
                        "id": "text-1",
                        "type": "mj-text",
                        "attributes": {
                          "align": "center",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "24px",
                          "fontWeight": "bold",
                          "lineHeight": "1.6",
                          "paddingTop": "30px",
                          "paddingRight": "25px",
                          "paddingBottom": "30px",
                          "paddingLeft": "25px"
                        },
                        "content": "<p>Confirm Reauthentication</p>"
                      },
                      {
                        "id": "text-2",
                        "type": "mj-text",
                        "attributes": {
                          "align": "left",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "16px",
                          "lineHeight": "1.6",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px"
                        },
                        "content": "<p>We need to verify your identity before proceeding with this sensitive action.</p><p></p><p>Please enter the following verification code:</p>"
                      },
                      {
                        "id": "text-token",
                        "type": "mj-text",
                        "attributes": {
                          "align": "center",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "26px",
                          "fontWeight": "bold",
                          "lineHeight": "1.6",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px",
                          "backgroundColor": "#f0f5ff"
                        },
                        "content": "<p>{{ email_data.token }}</p>"
                      },
                      {
                        "id": "text-3",
                        "type": "mj-text",
                        "attributes": {
                          "align": "left",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "16px",
                          "lineHeight": "1.6",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px"
                        },
                        "content": "<p>This code will expire in 10 minutes.</p><p></p><p>If you didn't request this verification, please ignore this email and contact support if you have concerns about your account security.</p>"
                      }
                    ]
                  }
                ]
              },
              {
                "id": "section-2",
                "type": "mj-section",
                "attributes": {
                  "backgroundColor": "transparent",
                  "paddingTop": "20px",
                  "paddingRight": "0px",
                  "paddingBottom": "20px",
                  "paddingLeft": "0px",
                  "textAlign": "center"
                },
                "children": [
                  {
                    "id": "column-2",
                    "type": "mj-column",
                    "attributes": {
                      "width": "100%"
                    },
                    "children": [
                      {
                        "id": "text-4",
                        "type": "mj-text",
                        "attributes": {
                          "align": "left",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "10px",
                          "lineHeight": "1.6",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px"
                        },
                        "content": "<p>Powered by <a class=\"editor-link\" href=\"https://www.notifuse.com\">Notifuse</a></p>"
                      }
                    ]
                  }
                ]
              }
            ]
          }
        ]
      }
    ]
  }
}`

	return parseEmailTreeJSON(jsonTemplate)
}

