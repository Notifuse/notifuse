package templates

import (
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// CreateSupabaseMagicLinkEmailStructure creates the detailed MJML structure for the magic link email
func CreateSupabaseMagicLinkEmailStructure() (notifuse_mjml.EmailBlock, error) {
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
                        "content": "<p>Your sign-in link</p>"
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
                        "content": "<p>Click the link below to sign in to your account:</p>"
                      },
                      {
                        "id": "button-1",
                        "type": "mj-button",
                        "attributes": {
                          "align": "center",
                          "backgroundColor": "#5850ec",
                          "borderRadius": "4px",
                          "color": "#ffffff",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "16px",
                          "fontWeight": "bold",
                          "href": "{{ email_data.site_url }}/verify?token={{ email_data.token_hash }}&type={{ email_data.email_action_type }}&redirect_to={{ email_data.redirect_to }}",
                          "innerPadding": "12px 24px",
                          "paddingTop": "15px",
                          "paddingRight": "25px",
                          "paddingBottom": "15px",
                          "paddingLeft": "25px"
                        },
                        "content": "Sign In"
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
                        "content": "<p>This link will expire in 1 hour and can only be used once.</p><p>If you didn't request this link, you can safely ignore this email.</p>"
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

