package templates

import (
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// CreateSupabaseSignupEmailStructure creates the detailed MJML structure for the signup confirmation email
func CreateSupabaseSignupEmailStructure() (notifuse_mjml.EmailBlock, error) {
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
        "children": [
          {
            "id": "attributes-1",
            "type": "mj-attributes",
            "attributes": {},
            "children": [
              {
                "id": "text-defaults-1",
                "type": "mj-text",
                "attributes": {
                  "align": "left",
                  "color": "#333333",
                  "fontFamily": "Arial, sans-serif",
                  "fontSize": "16px",
                  "fontStyle": "normal",
                  "fontWeight": "normal",
                  "lineHeight": "1.6",
                  "textDecoration": "none",
                  "textTransform": "none",
                  "paddingTop": "10px",
                  "paddingRight": "25px",
                  "paddingBottom": "10px",
                  "paddingLeft": "25px",
                  "backgroundColor": "transparent"
                }
              },
              {
                "id": "button-defaults-1",
                "type": "mj-button",
                "attributes": {
                  "align": "center",
                  "backgroundColor": "#007bff",
                  "borderRadius": "4px",
                  "border": "none",
                  "color": "#ffffff",
                  "fontFamily": "Arial, sans-serif",
                  "fontSize": "16px",
                  "fontStyle": "normal",
                  "fontWeight": "bold",
                  "href": "",
                  "innerPadding": "12px 24px",
                  "lineHeight": "120%",
                  "paddingTop": "15px",
                  "paddingRight": "25px",
                  "paddingBottom": "15px",
                  "paddingLeft": "25px",
                  "target": "_blank",
                  "textDecoration": "none",
                  "textTransform": "none",
                  "verticalAlign": "middle",
                  "rel": "noopener noreferrer"
                }
              },
              {
                "id": "image-defaults-1",
                "type": "mj-image",
                "attributes": {
                  "align": "center",
                  "alt": "",
                  "border": "none",
                  "borderRadius": "0px",
                  "containerBackgroundColor": "transparent",
                  "fluidOnMobile": "false",
                  "height": "auto",
                  "href": "",
                  "paddingTop": "10px",
                  "paddingRight": "25px",
                  "paddingBottom": "10px",
                  "paddingLeft": "25px",
                  "rel": "noopener noreferrer",
                  "sizes": "",
                  "src": "https://placehold.co/150x60/E3F2FD/1976D2?font=playfair-display&text=LOGO",
                  "srcset": "",
                  "target": "_blank",
                  "title": "",
                  "usemap": "",
                  "width": "150px"
                }
              },
              {
                "id": "section-defaults-1",
                "type": "mj-section",
                "attributes": {
                  "backgroundColor": "transparent",
                  "backgroundUrl": "",
                  "backgroundRepeat": "no-repeat",
                  "backgroundSize": "auto",
                  "backgroundPosition": "top center",
                  "border": "none",
                  "borderTop": "none",
                  "borderRight": "none",
                  "borderBottom": "none",
                  "borderLeft": "none",
                  "borderRadius": "0px",
                  "direction": "ltr",
                  "fullWidth": "full-width",
                  "paddingTop": "20px",
                  "paddingRight": "0px",
                  "paddingBottom": "20px",
                  "paddingLeft": "0px",
                  "textAlign": "center",
                  "cssClass": ""
                },
                "children": []
              },
              {
                "id": "column-defaults-1",
                "type": "mj-column",
                "attributes": {
                  "width": "100%",
                  "verticalAlign": "top",
                  "backgroundColor": "transparent",
                  "innerBackgroundColor": "transparent",
                  "border": "none",
                  "borderTop": "none",
                  "borderRight": "none",
                  "borderBottom": "none",
                  "borderLeft": "none",
                  "borderRadius": "0px",
                  "innerBorder": "none",
                  "innerBorderTop": "none",
                  "innerBorderRight": "none",
                  "innerBorderBottom": "none",
                  "innerBorderLeft": "none",
                  "innerBorderRadius": "0px",
                  "paddingTop": "0px",
                  "paddingRight": "0px",
                  "paddingBottom": "0px",
                  "paddingLeft": "0px",
                  "cssClass": ""
                },
                "children": []
              }
            ]
          },
          {
            "id": "preview-1",
            "type": "mj-preview",
            "attributes": {
              "content": ""
            },
            "content": "Welcome! See how sections provide borders and groups prevent mobile stacking."
          }
        ]
      },
      {
        "id": "body-1",
        "type": "mj-body",
        "attributes": {
          "width": "600px",
          "backgroundColor": "#f7f8fa"
        },
        "children": [
          {
            "id": "wrapper-1",
            "type": "mj-wrapper",
            "attributes": {
              "paddingTop": "20px",
              "paddingRight": "20px",
              "paddingBottom": "20px",
              "paddingLeft": "20px",
              "backgroundColor": "transparent"
            },
            "children": [
              {
                "id": "f24eb926-19cc-43af-9c9b-2b7b16c88522",
                "type": "mj-section",
                "attributes": {
                  "backgroundColor": "transparent",
                  "backgroundUrl": "",
                  "backgroundRepeat": "no-repeat",
                  "backgroundSize": "auto",
                  "backgroundPosition": "top center",
                  "border": "none",
                  "borderTop": "none",
                  "borderRight": "none",
                  "borderBottom": "none",
                  "borderLeft": "none",
                  "borderRadius": "0px",
                  "direction": "ltr",
                  "fullWidth": "full-width",
                  "paddingTop": "0px",
                  "paddingRight": "0px",
                  "paddingBottom": "20px",
                  "paddingLeft": "0px",
                  "textAlign": "center",
                  "cssClass": ""
                },
                "children": [
                  {
                    "id": "3225d599-bf39-408f-bdca-27b1d054dba1",
                    "type": "mj-column",
                    "attributes": {
                      "width": "100%",
                      "verticalAlign": "top",
                      "backgroundColor": "transparent",
                      "innerBackgroundColor": "transparent",
                      "border": "none",
                      "borderTop": "none",
                      "borderRight": "none",
                      "borderBottom": "none",
                      "borderLeft": "none",
                      "borderRadius": "0px",
                      "innerBorder": "none",
                      "innerBorderTop": "none",
                      "innerBorderRight": "none",
                      "innerBorderBottom": "none",
                      "innerBorderLeft": "none",
                      "innerBorderRadius": "0px",
                      "paddingTop": "0px",
                      "paddingRight": "0px",
                      "paddingBottom": "0px",
                      "paddingLeft": "0px",
                      "cssClass": ""
                    },
                    "children": [
                      {
                        "id": "c52cd1ec-81de-4917-8e42-bc3f61444f9c",
                        "type": "mj-image",
                        "attributes": {
                          "align": "center",
                          "alt": "",
                          "border": "none",
                          "borderRadius": "0px",
                          "containerBackgroundColor": "transparent",
                          "fluidOnMobile": "false",
                          "height": "auto",
                          "href": "",
                          "paddingTop": "10px",
                          "paddingRight": "0px",
                          "paddingBottom": "10px",
                          "paddingLeft": "0px",
                          "rel": "noopener noreferrer",
                          "sizes": "",
                          "src": "https://storage.googleapis.com/readonlydemo/supabase-notifuse.png",
                          "srcset": "",
                          "target": "_blank",
                          "title": "",
                          "usemap": "",
                          "width": "160px"
                        }
                      }
                    ]
                  }
                ]
              },
              {
                "id": "ce93d36b-a0a9-4a3c-b40f-8270881fd605",
                "type": "mj-section",
                "attributes": {
                  "backgroundColor": "#ffffff",
                  "backgroundUrl": "",
                  "backgroundRepeat": "no-repeat",
                  "backgroundSize": "auto",
                  "backgroundPosition": "top center",
                  "border": "none",
                  "borderTop": "none",
                  "borderRight": "none",
                  "borderBottom": "none",
                  "borderLeft": "none",
                  "borderRadius": "8px",
                  "direction": "ltr",
                  "fullWidth": "full-width",
                  "paddingTop": "0px",
                  "paddingRight": "0px",
                  "paddingBottom": "20px",
                  "paddingLeft": "0px",
                  "textAlign": "center",
                  "cssClass": ""
                },
                "children": [
                  {
                    "id": "0625917a-fee8-40a1-ab71-ff1fbdf1704d",
                    "type": "mj-column",
                    "attributes": {
                      "width": "100%",
                      "verticalAlign": "top",
                      "backgroundColor": "transparent",
                      "innerBackgroundColor": "transparent",
                      "border": "none",
                      "borderTop": "none",
                      "borderRight": "none",
                      "borderBottom": "none",
                      "borderLeft": "none",
                      "borderRadius": "0px",
                      "innerBorder": "none",
                      "innerBorderTop": "none",
                      "innerBorderRight": "none",
                      "innerBorderBottom": "none",
                      "innerBorderLeft": "none",
                      "innerBorderRadius": "0px",
                      "paddingTop": "0px",
                      "paddingRight": "0px",
                      "paddingBottom": "0px",
                      "paddingLeft": "0px",
                      "cssClass": ""
                    },
                    "children": [
                      {
                        "id": "e4b7bbe1-212c-4672-8603-b3804dd4802a",
                        "type": "mj-text",
                        "attributes": {
                          "align": "left",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "24px",
                          "fontStyle": "normal",
                          "fontWeight": "bold",
                          "lineHeight": "30px",
                          "textDecoration": "none",
                          "textTransform": "none",
                          "paddingTop": "25px",
                          "paddingRight": "25px",
                          "paddingBottom": "25px",
                          "paddingLeft": "25px",
                          "backgroundColor": "transparent"
                        },
                        "content": "<p>Confirm your email address</p>"
                      },
                      {
                        "id": "86e80ba3-923d-432b-8ef0-5b6c41a2cab1",
                        "type": "mj-text",
                        "attributes": {
                          "align": "left",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "16px",
                          "fontStyle": "normal",
                          "fontWeight": "normal",
                          "lineHeight": "1.6",
                          "textDecoration": "none",
                          "textTransform": "none",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px",
                          "backgroundColor": "transparent"
                        },
                        "content": "<p>Welcome! Please confirm your email address to complete your registration.<br><br>Click the button below to verify your email:</p>"
                      },
                      {
                        "id": "65e4253a-ed84-4fa4-a5b3-8f4026f09c47",
                        "type": "mj-button",
                        "attributes": {
                          "align": "center",
                          "backgroundColor": "#5850ec",
                          "borderRadius": "4px",
                          "border": "none",
                          "color": "#ffffff",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "16px",
                          "fontStyle": "normal",
                          "fontWeight": "bold",
                          "href": "{{ email_data.site_url }}/verify?token={{ email_data.token_hash }}&type={{ email_data.email_action_type }}&redirect_to={{ email_data.redirect_to }}",
                          "innerPadding": "12px 24px",
                          "lineHeight": "120%",
                          "paddingTop": "15px",
                          "paddingRight": "25px",
                          "paddingBottom": "15px",
                          "paddingLeft": "25px",
                          "target": "_blank",
                          "textDecoration": "none",
                          "textTransform": "none",
                          "verticalAlign": "middle",
                          "rel": "noopener noreferrer"
                        },
                        "content": "Confirm Email"
                      },
                      {
                        "id": "84927c4f-ef7a-4ed8-95e4-c0fd67ebcc14",
                        "type": "mj-text",
                        "attributes": {
                          "align": "left",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "16px",
                          "fontStyle": "normal",
                          "fontWeight": "normal",
                          "lineHeight": "1.6",
                          "textDecoration": "none",
                          "textTransform": "none",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px",
                          "backgroundColor": "transparent"
                        },
                        "content": "<p>This link will expire in 24 hours.<br><br>If you didn't create an account, you can safely ignore this email.</p>"
                      }
                    ]
                  }
                ]
              },
              {
                "id": "ea19db55-35eb-424b-bf6d-844cfe6f9a93",
                "type": "mj-section",
                "attributes": {
                  "backgroundColor": "transparent",
                  "backgroundUrl": "",
                  "backgroundRepeat": "no-repeat",
                  "backgroundSize": "auto",
                  "backgroundPosition": "top center",
                  "border": "none",
                  "borderTop": "none",
                  "borderRight": "none",
                  "borderBottom": "none",
                  "borderLeft": "none",
                  "borderRadius": "0px",
                  "direction": "ltr",
                  "fullWidth": "full-width",
                  "paddingTop": "20px",
                  "paddingRight": "0px",
                  "paddingBottom": "20px",
                  "paddingLeft": "0px",
                  "textAlign": "center",
                  "cssClass": ""
                },
                "children": [
                  {
                    "id": "76a901c0-d1d1-4741-929d-247ffe08e321",
                    "type": "mj-column",
                    "attributes": {
                      "width": "100%",
                      "verticalAlign": "top",
                      "backgroundColor": "transparent",
                      "innerBackgroundColor": "transparent",
                      "border": "none",
                      "borderTop": "none",
                      "borderRight": "none",
                      "borderBottom": "none",
                      "borderLeft": "none",
                      "borderRadius": "0px",
                      "innerBorder": "none",
                      "innerBorderTop": "none",
                      "innerBorderRight": "none",
                      "innerBorderBottom": "none",
                      "innerBorderLeft": "none",
                      "innerBorderRadius": "0px",
                      "paddingTop": "0px",
                      "paddingRight": "0px",
                      "paddingBottom": "0px",
                      "paddingLeft": "0px",
                      "cssClass": ""
                    },
                    "children": [
                      {
                        "id": "b90da51c-ac2c-45f6-b763-166ea0e6ab3f",
                        "type": "mj-text",
                        "attributes": {
                          "align": "center",
                          "color": "#333333",
                          "fontFamily": "Arial, sans-serif",
                          "fontSize": "10px",
                          "fontStyle": "normal",
                          "fontWeight": "normal",
                          "lineHeight": "1.6",
                          "textDecoration": "none",
                          "textTransform": "none",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px",
                          "backgroundColor": "transparent"
                        },
                        "content": "<p>Please do not reply to this email.<br>Need help? visit help center or contact us.<br>12 Heaven Road | San Francisco CA<br>Powered by <a class=\"editor-link\" href=\"https://www.notifuse.com\">Notifuse</a></p>"
                      },
                      {
                        "id": "31cc03f4-74b6-4333-ad9f-0bb913ae93ee",
                        "type": "mj-social",
                        "attributes": {
                          "align": "center",
                          "borderRadius": "3px",
                          "containerBackgroundColor": "transparent",
                          "iconHeight": "20px",
                          "iconSize": "20px",
                          "innerPadding": "4px",
                          "lineHeight": "22px",
                          "mode": "horizontal",
                          "paddingTop": "10px",
                          "paddingRight": "25px",
                          "paddingBottom": "10px",
                          "paddingLeft": "25px",
                          "tableLayout": "auto",
                          "textPadding": "4px 4px 4px 0px"
                        },
                        "children": [
                          {
                            "id": "4a678780-a49f-4363-839b-c3dc8788221e",
                            "type": "mj-social-element",
                            "attributes": {
                              "align": "center",
                              "alt": "",
                              "backgroundColor": "#000000",
                              "borderRadius": "3px",
                              "color": "#333333",
                              "fontFamily": "Ubuntu, Helvetica, Arial, sans-serif",
                              "fontSize": "13px",
                              "fontStyle": "normal",
                              "fontWeight": "normal",
                              "iconSize": "20px",
                              "iconPadding": "0px",
                              "lineHeight": "22px",
                              "padding": "4px",
                              "target": "_blank",
                              "textDecoration": "none",
                              "textPadding": "4px 4px 4px 0",
                              "verticalAlign": "middle",
                              "name": "github",
                              "href": "https://github.com/Notifuse/notifuse"
                            },
                            "children": []
                          },
                          {
                            "id": "1d8a1888-1e3f-41ba-ba94-84387cbbf799",
                            "type": "mj-social-element",
                            "attributes": {
                              "name": "x",
                              "href": "https://x.com/notifuse",
                              "backgroundColor": "#000000",
                              "borderRadius": "3px"
                            },
                            "children": []
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
    ]
  },
  "testData": {
    "email_data": {
      "token": "123456"
    }
  },
  "exportedAt": "2025-11-03T10:21:03.524Z",
  "version": "1.0"
}`

	return parseEmailTreeJSON(jsonTemplate)
}
