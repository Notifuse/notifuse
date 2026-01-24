import React from 'react'
import { Modal, Button, Alert, Tabs, Descriptions } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCopy } from '@fortawesome/free-solid-svg-icons'
import { Highlight, themes } from 'prism-react-renderer'
import { TransactionalNotification } from '../../services/api/transactional_notifications'
import { message } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface ApiCommandModalProps {
  open: boolean
  onClose: () => void
  notification: TransactionalNotification | null
  workspaceId: string
}

export const ApiCommandModal: React.FC<ApiCommandModalProps> = ({
  open,
  onClose,
  notification,
  workspaceId
}) => {
  const { t } = useLingui()
  const generateCurlCommand = () => {
    if (!notification) return ''

    return `curl -X POST \\
  "${window.API_ENDPOINT}/api/transactional.send" \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
  "workspace_id": "${workspaceId}",
  "notification": {
    "id": "${notification.id}",
    "external_id": "your-unique-id-123",
    "channels": ["email"],
    "contact": {
      "email": "recipient@example.com",
      "first_name": "John",
      "last_name": "Doe",
      "full_name": "John Doe",
      "external_id": "user-123",
      "timezone": "America/New_York",
      "language": "en",
      "phone": "+1234567890",
      "address_line_1": "123 Main St",
      "address_line_2": "Apt 4B",
      "country": "US",
      "postcode": "10001",
      "state": "NY",
      "job_title": "Software Engineer",
      "custom_string_1": "custom_value_1",
      "custom_string_2": "custom_value_2",
      "custom_number_1": 42.5,
      "custom_number_2": 100,
      "custom_datetime_1": "2024-01-10T14:30:00Z",
      "custom_datetime_2": "2023-12-25T00:00:00Z",
      "custom_json_1": { "preferences": ["email", "sms"], "tier": "premium" },
      "custom_json_2": { "last_purchase": { "product": "Pro Plan", "amount": 99.99 } }
    },
    "data": {
      "your_template_variable": "value",
      "product_name": "Premium Plan",
      "amount": "$99.99",
      "discount_code": "WELCOME20"
    },
    "metadata": {
      "campaign_id": "welcome-series",
      "source": "website",
      "user_segment": "premium",
      "internal_note": "High-value customer"
    },
    "email_options": {
      "reply_to": "support@example.com",
      "cc": ["manager@example.com"],
      "bcc": ["audit@example.com"]
    }
  }
}'`
  }

  const generateTypeScriptCode = () => {
    if (!notification) return ''

    return `interface Contact {
  // Required field
  email: string;
  
  // Optional fields
  external_id?: string;
  timezone?: string;
  language?: string;
  first_name?: string;
  last_name?: string;
  full_name?: string;
  phone?: string;
  address_line_1?: string;
  address_line_2?: string;
  country?: string;
  postcode?: string;
  state?: string;
  job_title?: string;

  // Custom string fields
  custom_string_1?: string;
  custom_string_2?: string;
  custom_string_3?: string;
  custom_string_4?: string;
  custom_string_5?: string;
  
  // Custom number fields
  custom_number_1?: number;
  custom_number_2?: number;
  custom_number_3?: number;
  custom_number_4?: number;
  custom_number_5?: number;
  
  // Custom datetime fields
  custom_datetime_1?: string;
  custom_datetime_2?: string;
  custom_datetime_3?: string;
  custom_datetime_4?: string;
  custom_datetime_5?: string;
  
  // Custom JSON fields
  custom_json_1?: any;
  custom_json_2?: any;
  custom_json_3?: any;
  custom_json_4?: any;
  custom_json_5?: any;
}

interface EmailOptions {
  reply_to?: string;
  cc?: string[];
  bcc?: string[];
}

interface NotificationRequest {
  workspace_id: string;
  notification: {
    id: string;
    external_id?: string; // For deduplication
    channels?: string[];
    contact: Contact;
    data?: Record<string, any>; // Template variables for rendering
    metadata?: Record<string, any>; // Tracking data (not used in templates)
    email_options?: EmailOptions;
  };
}

const sendNotification = async (): Promise<void> => {
  const payload: NotificationRequest = {
    workspace_id: "${workspaceId}",
    notification: {
      id: "${notification.id}",
      external_id: "your-unique-id-123", // For deduplication
      channels: ["email"],
      contact: {
        email: "recipient@example.com",
        first_name: "John",
        last_name: "Doe",
        full_name: "John Doe",
        external_id: "user-123",
        timezone: "America/New_York",
        language: "en",
        phone: "+1234567890",
        address_line_1: "123 Main St",
        address_line_2: "Apt 4B",
        country: "US",
        postcode: "10001",
        state: "NY",
        job_title: "Software Engineer",
        custom_string_1: "custom_value_1",
        custom_string_2: "custom_value_2",
        custom_number_1: 42.5,
        custom_number_2: 100,
        custom_datetime_1: "2024-01-10T14:30:00Z",
        custom_datetime_2: "2023-12-25T00:00:00Z",
        custom_json_1: { "preferences": ["email", "sms"], "tier": "premium" },
        custom_json_2: { "last_purchase": { "product": "Pro Plan", "amount": 99.99 } }
      },
      data: {
        your_template_variable: "value",
        product_name: "Premium Plan",
        amount: "$99.99",
        discount_code: "WELCOME20"
      },
      metadata: {
        campaign_id: "welcome-series",
        source: "website",
        user_segment: "premium",
        internal_note: "High-value customer"
      },
      email_options: {
        reply_to: "support@example.com",
        cc: ["manager@example.com"],
        bcc: ["audit@example.com"]
      }
    }
  };

  try {
    const response = await fetch("${window.API_ENDPOINT}/api/transactional.send", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": "Bearer YOUR_API_KEY"
      },
      body: JSON.stringify(payload)
    });

    if (!response.ok) {
      throw new Error(\`HTTP error! status: \${response.status}\`);
    }

    const result = await response.json();
    console.log("Notification sent successfully:", result);
  } catch (error) {
    console.error("Error sending notification:", error);
  }
};

// Call the function
sendNotification();`
  }

  const generatePythonCode = () => {
    if (!notification) return ''

    return `import requests
import json

def send_notification():
    url = "${window.API_ENDPOINT}/api/transactional.send"
    
    headers = {
        "Content-Type": "application/json",
        "Authorization": "Bearer YOUR_API_KEY"
    }
    
    payload = {
        "workspace_id": "${workspaceId}",
        "notification": {
            "id": "${notification.id}",
            "external_id": "your-unique-id-123",  # For deduplication
            "channels": ["email"],
            "contact": {
                "email": "recipient@example.com",
                "first_name": "John",
                "last_name": "Doe",
                "full_name": "John Doe",
                "external_id": "user-123",
                "timezone": "America/New_York",
                "language": "en",
                "phone": "+1234567890",
                "address_line_1": "123 Main St",
                "address_line_2": "Apt 4B",
                "country": "US",
                "postcode": "10001",
                "state": "NY",
                "job_title": "Software Engineer",
                "custom_string_1": "custom_value_1",
                "custom_string_2": "custom_value_2",
                "custom_number_1": 42.5,
                "custom_number_2": 100,
                "custom_datetime_1": "2024-01-10T14:30:00Z",
                "custom_datetime_2": "2023-12-25T00:00:00Z",
                "custom_json_1": { "preferences": ["email", "sms"], "tier": "premium" },
                "custom_json_2": { "last_purchase": { "product": "Pro Plan", "amount": 99.99 } },
                "custom_number_1": 42.5,
                "custom_number_2": 100,
                "custom_datetime_1": "2024-01-10T14:30:00Z",
                "custom_datetime_2": "2023-12-25T00:00:00Z",
                "custom_json_1": { "preferences": ["email", "sms"], "tier": "premium" },
                "custom_json_2": { "last_purchase": { "product": "Pro Plan", "amount": 99.99 } }
            },
            "data": {
                "your_template_variable": "value",
                "product_name": "Premium Plan",
                "amount": "$99.99",
                "discount_code": "WELCOME20"
            },
            "metadata": {
                "campaign_id": "welcome-series",
                "source": "website",
                "user_segment": "premium",
                "internal_note": "High-value customer"
            },
            "email_options": {
                "reply_to": "support@example.com",
                "cc": ["manager@example.com"],
                "bcc": ["audit@example.com"]
            }
        }
    }
    
    try:
        response = requests.post(url, headers=headers, json=payload)
        response.raise_for_status()  # Raises an HTTPError for bad responses
        
        result = response.json()
        print("Notification sent successfully:", result)
        return result
        
    except requests.exceptions.RequestException as e:
        print(f"Error sending notification: {e}")
        return None

# Call the function
send_notification()`
  }

  const generateGolangCode = () => {
    if (!notification) return ''

    return `package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

type Contact struct {
    Email         string   \`json:"email"\`
    ExternalID    string   \`json:"external_id,omitempty"\`
    Timezone      string   \`json:"timezone,omitempty"\`
    Language      string   \`json:"language,omitempty"\`
    FirstName     string   \`json:"first_name,omitempty"\`
    LastName      string   \`json:"last_name,omitempty"\`
    FullName      string   \`json:"full_name,omitempty"\`
    Phone         string   \`json:"phone,omitempty"\`
    AddressLine1  string   \`json:"address_line_1,omitempty"\`
    AddressLine2  string   \`json:"address_line_2,omitempty"\`
    Country       string   \`json:"country,omitempty"\`
    Postcode      string   \`json:"postcode,omitempty"\`
    State         string   \`json:"state,omitempty"\`
    JobTitle      string   \`json:"job_title,omitempty"\`
    CustomString1 string   \`json:"custom_string_1,omitempty"\`
    CustomString2 string   \`json:"custom_string_2,omitempty"\`
    CustomString3 string   \`json:"custom_string_3,omitempty"\`
    CustomString4 string   \`json:"custom_string_4,omitempty"\`
    CustomString5 string   \`json:"custom_string_5,omitempty"\`
    
    CustomNumber1 *float64 \`json:"custom_number_1,omitempty"\`
    CustomNumber2 *float64 \`json:"custom_number_2,omitempty"\`
    CustomNumber3 *float64 \`json:"custom_number_3,omitempty"\`
    CustomNumber4 *float64 \`json:"custom_number_4,omitempty"\`
    CustomNumber5 *float64 \`json:"custom_number_5,omitempty"\`
    
    CustomDatetime1 *string \`json:"custom_datetime_1,omitempty"\`
    CustomDatetime2 *string \`json:"custom_datetime_2,omitempty"\`
    CustomDatetime3 *string \`json:"custom_datetime_3,omitempty"\`
    CustomDatetime4 *string \`json:"custom_datetime_4,omitempty"\`
    CustomDatetime5 *string \`json:"custom_datetime_5,omitempty"\`
    
    CustomJSON1 interface{} \`json:"custom_json_1,omitempty"\`
    CustomJSON2 interface{} \`json:"custom_json_2,omitempty"\`
    CustomJSON3 interface{} \`json:"custom_json_3,omitempty"\`
    CustomJSON4 interface{} \`json:"custom_json_4,omitempty"\`
    CustomJSON5 interface{} \`json:"custom_json_5,omitempty"\`
}

type EmailOptions struct {
    ReplyTo string   \`json:"reply_to,omitempty"\`
    CC      []string \`json:"cc,omitempty"\`
    BCC     []string \`json:"bcc,omitempty"\`
}

type Notification struct {
    ID           string                 \`json:"id"\`
    ExternalID   *string                \`json:"external_id,omitempty"\`
    Channels     []string               \`json:"channels,omitempty"\`
    Contact      Contact                \`json:"contact"\`
    Data         map[string]interface{} \`json:"data,omitempty"\`
    Metadata     map[string]interface{} \`json:"metadata,omitempty"\`
    EmailOptions *EmailOptions          \`json:"email_options,omitempty"\`
}

type NotificationRequest struct {
    WorkspaceID  string       \`json:"workspace_id"\`
    Notification Notification \`json:"notification"\`
}

func sendNotification() error {
    url := "${window.API_ENDPOINT}/api/transactional.send"
    
    externalID := "your-unique-id-123"

    payload := NotificationRequest{
        WorkspaceID: "${workspaceId}",
        Notification: Notification{
            ID:         "${notification.id}",
            ExternalID: &externalID, // For deduplication
            Channels:   []string{"email"},
            Contact: Contact{
                Email:         "recipient@example.com",
                FirstName:     "John",
                LastName:      "Doe",
                FullName:      "John Doe",
                ExternalID:    "user-123",
                Timezone:      "America/New_York",
                Language:      "en",
                Phone:         "+1234567890",
                AddressLine1:  "123 Main St",
                AddressLine2:  "Apt 4B",
                Country:       "US",
                Postcode:      "10001",
                State:         "NY",
                JobTitle:      "Software Engineer",
                CustomString1: "custom_value_1",
                CustomString2: "custom_value_2",
                CustomNumber1: func() *float64 { v := 42.5; return &v }(),
                CustomNumber2: func() *float64 { v := 100.0; return &v }(),
                CustomDatetime1: func() *string { v := "2024-01-10T14:30:00Z"; return &v }(),
                CustomDatetime2: func() *string { v := "2023-12-25T00:00:00Z"; return &v }(),
                // Note: Custom JSON fields would be set separately in Go
            },
            Data: map[string]interface{}{
                "your_template_variable": "value",
                "product_name":           "Premium Plan",
                "amount":                 "$99.99",
                "discount_code":          "WELCOME20",
            },
            Metadata: map[string]interface{}{
                "campaign_id":     "welcome-series",
                "source":          "website",
                "user_segment":    "premium",
                "internal_note":   "High-value customer",
            },
            EmailOptions: &EmailOptions{
                ReplyTo: "support@example.com",
                CC:      []string{"manager@example.com"},
                BCC:     []string{"audit@example.com"},
            },
        },
    }
    
    jsonData, err := json.Marshal(payload)
    if err != nil {
        return fmt.Errorf("error marshaling JSON: %w", err)
    }
    
    req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        return fmt.Errorf("error creating request: %w", err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer YOUR_API_KEY")
    
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("error sending request: %w", err)
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("error reading response: %w", err)
    }
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("HTTP error! status: %d, body: %s", resp.StatusCode, string(body))
    }
    
    fmt.Printf("Notification sent successfully: %s\\n", string(body))
    return nil
}

func main() {
    if err := sendNotification(); err != nil {
        fmt.Printf("Error: %v\\n", err)
    }
}`
  }

  const generateSMTPPayload = () => {
    if (!notification) return ''

    return `{
  "workspace_id": "${workspaceId}",
  "notification": {
    "id": "${notification.id}",
    "external_id": "your-unique-id-123",
    "channels": ["email"],
    "contact": {
      "email": "recipient@example.com",
      "first_name": "John",
      "last_name": "Doe",
      "full_name": "John Doe",
      "external_id": "user-123",
      "timezone": "America/New_York",
      "language": "en"
    },
    "data": {
      "your_template_variable": "value",
      "product_name": "Premium Plan",
      "amount": "$99.99",
      "discount_code": "WELCOME20"
    },
    "email_options": {
      "reply_to": "support@example.com",
      "cc": ["manager@example.com"],
      "bcc": ["audit@example.com"]
    }
  }
}`
  }

  const renderSMTPInstructions = () => {
    const smtpHost = window.SMTP_RELAY_DOMAIN || 'your-smtp-domain.com'
    const smtpPort = window.SMTP_RELAY_PORT || 587
    const tlsEnabled = window.SMTP_RELAY_TLS_ENABLED !== false

    return (
      <div className="space-y-6">
        <div>
          <h3 className="text-base font-semibold mb-3">{t`Connection Details`}</h3>
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label={t`Host`}>{smtpHost}</Descriptions.Item>
            <Descriptions.Item label={t`Port`}>{smtpPort}</Descriptions.Item>
            <Descriptions.Item label={t`Security`}>
              {tlsEnabled ? t`STARTTLS required` : t`Plain text (not recommended for production)`}
            </Descriptions.Item>
            <Descriptions.Item label={t`Username`}>
              {t`Your workspace API email (the email associated with your API key)`}
            </Descriptions.Item>
            <Descriptions.Item label={t`Password`}>{t`Your workspace API key`}</Descriptions.Item>
          </Descriptions>
        </div>

        <div>
          <h3 className="text-base font-semibold mb-3">{t`Email Body Payload`}</h3>
          <p className="mb-3 text-sm">
            {t`The email body must contain a JSON payload with your notification data. The SMTP envelope To/From addresses are ignored - the actual recipient is determined by`}{' '}
            <code>contact.email</code> {t`in the payload.`}
          </p>
          <CodeBlock code={generateSMTPPayload()} language="json" />
        </div>

        <div>
          <h3 className="text-base font-semibold mb-3">{t`Important Notes`}</h3>
          <ul className="list-disc list-inside space-y-1 text-sm">
            <li>
              <strong>{t`JSON Payload Required:`}</strong> {t`The email body must contain valid JSON matching the format above`}
            </li>
            <li>
              <strong>{t`Contact Email:`}</strong> {t`The`} <code>contact.email</code> {t`field is required`}
            </li>
            <li>
              <strong>{t`Deduplication:`}</strong> {t`Use`} <code>external_id</code> {t`to prevent duplicate sends`}
            </li>
            <li>
              <strong>{t`Template Variables:`}</strong> {t`Use`} <code>data</code> {t`for template variables`}
            </li>
            <li>
              <strong>{t`Email Options:`}</strong> {t`Supports reply_to, cc, bcc, and attachments`}
            </li>
          </ul>
        </div>
      </div>
    )
  }

  const generateJavaCode = () => {
    if (!notification) return ''

    return `import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.annotation.JsonProperty;

public class NotificationSender {
    
    public static class Contact {
        public String email;
        @JsonProperty("external_id")
        public String externalId;
        public String timezone;
        public String language;
        @JsonProperty("first_name")
        public String firstName;
        @JsonProperty("last_name")
        public String lastName;
        @JsonProperty("full_name")
        public String fullName;
        public String phone;
        @JsonProperty("address_line_1")
        public String addressLine1;
        @JsonProperty("address_line_2")
        public String addressLine2;
        public String country;
        public String postcode;
        public String state;
        @JsonProperty("job_title")
        public String jobTitle;
        @JsonProperty("custom_string_1")
        public String customString1;
        @JsonProperty("custom_string_2")
        public String customString2;
        @JsonProperty("custom_string_3")
        public String customString3;
        @JsonProperty("custom_string_4")
        public String customString4;
        @JsonProperty("custom_string_5")
        public String customString5;
        
        // Custom number fields
        @JsonProperty("custom_number_1")
        public Double customNumber1;
        @JsonProperty("custom_number_2")
        public Double customNumber2;
        @JsonProperty("custom_number_3")
        public Double customNumber3;
        @JsonProperty("custom_number_4")
        public Double customNumber4;
        @JsonProperty("custom_number_5")
        public Double customNumber5;
        
        // Custom datetime fields
        @JsonProperty("custom_datetime_1")
        public String customDatetime1;
        @JsonProperty("custom_datetime_2")
        public String customDatetime2;
        @JsonProperty("custom_datetime_3")
        public String customDatetime3;
        @JsonProperty("custom_datetime_4")
        public String customDatetime4;
        @JsonProperty("custom_datetime_5")
        public String customDatetime5;
        
        // Custom JSON fields
        @JsonProperty("custom_json_1")
        public Object customJson1;
        @JsonProperty("custom_json_2")
        public Object customJson2;
        @JsonProperty("custom_json_3")
        public Object customJson3;
        @JsonProperty("custom_json_4")
        public Object customJson4;
        @JsonProperty("custom_json_5")
        public Object customJson5;
        
        public Contact(String email, String firstName, String lastName) {
            this.email = email;
            this.firstName = firstName;
            this.lastName = lastName;
        }
    }
    
    public static class EmailOptions {
        @JsonProperty("reply_to")
        public String replyTo;
        public String[] cc;
        public String[] bcc;
        
        public EmailOptions(String replyTo, String[] cc, String[] bcc) {
            this.replyTo = replyTo;
            this.cc = cc;
            this.bcc = bcc;
        }
    }
    
    public static class Notification {
        public String id;
        @JsonProperty("external_id")
        public String externalId;
        public String[] channels;
        public Contact contact;
        public Object data;
        public Object metadata;
        @JsonProperty("email_options")
        public EmailOptions emailOptions;
        
        public Notification(String id, String externalId, String[] channels, Contact contact, 
                          Object data, Object metadata, EmailOptions emailOptions) {
            this.id = id;
            this.externalId = externalId;
            this.channels = channels;
            this.contact = contact;
            this.data = data;
            this.metadata = metadata;
            this.emailOptions = emailOptions;
        }
    }
    
    public static class NotificationRequest {
        @JsonProperty("workspace_id")
        public String workspaceId;
        public Notification notification;
        
        public NotificationRequest(String workspaceId, Notification notification) {
            this.workspaceId = workspaceId;
            this.notification = notification;
        }
    }
    
    public static void sendNotification() throws IOException, InterruptedException {
        String url = "${window.API_ENDPOINT}/api/transactional.send";
        
        // Create the payload
        Contact contact = new Contact("recipient@example.com", "John", "Doe");
        contact.externalId = "user-123";
        contact.timezone = "America/New_York";
        contact.language = "en";
        contact.fullName = "John Doe";
        contact.phone = "+1234567890";
        contact.addressLine1 = "123 Main St";
        contact.addressLine2 = "Apt 4B";
        contact.country = "US";
        contact.postcode = "10001";
        contact.state = "NY";
        contact.jobTitle = "Software Engineer";
        contact.customString1 = "custom_value_1";
        contact.customString2 = "custom_value_2";
        contact.customNumber1 = 42.5;
        contact.customNumber2 = 100.0;
        contact.customDatetime1 = "2024-01-10T14:30:00Z";
        contact.customDatetime2 = "2023-12-25T00:00:00Z";
        
        // Custom JSON fields
        java.util.Map<String, Object> preferences = new java.util.HashMap<>();
        preferences.put("preferences", java.util.Arrays.asList("email", "sms"));
        preferences.put("tier", "premium");
        contact.customJson1 = preferences;
        
        java.util.Map<String, Object> lastPurchase = new java.util.HashMap<>();
        java.util.Map<String, Object> purchaseData = new java.util.HashMap<>();
        purchaseData.put("product", "Pro Plan");
        purchaseData.put("amount", 99.99);
        lastPurchase.put("last_purchase", purchaseData);
        contact.customJson2 = lastPurchase;
        
        EmailOptions emailOptions = new EmailOptions(
            "support@example.com",
            new String[]{"manager@example.com"},
            new String[]{"audit@example.com"}
        );
        
        java.util.Map<String, Object> data = new java.util.HashMap<>();
        data.put("your_template_variable", "value");
        data.put("product_name", "Premium Plan");
        data.put("amount", "$99.99");
        data.put("discount_code", "WELCOME20");
        
        java.util.Map<String, Object> metadata = new java.util.HashMap<>();
        metadata.put("campaign_id", "welcome-series");
        metadata.put("source", "website");
        metadata.put("user_segment", "premium");
        metadata.put("internal_note", "High-value customer");
        
        Notification notification = new Notification(
            "${notification.id}",
            "your-unique-id-123", // external_id for deduplication
            new String[]{"email"},
            contact,
            data,
            metadata,
            emailOptions
        );
        
        NotificationRequest request = new NotificationRequest("${workspaceId}", notification);
        
        // Convert to JSON
        ObjectMapper mapper = new ObjectMapper();
        String jsonString = mapper.writeValueAsString(request);
        
        // Create HTTP request
        HttpRequest httpRequest = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .timeout(Duration.ofMinutes(1))
            .header("Content-Type", "application/json")
            .header("Authorization", "Bearer YOUR_API_KEY")
            .POST(HttpRequest.BodyPublishers.ofString(jsonString))
            .build();
        
        // Send request
        HttpClient client = HttpClient.newHttpClient();
        HttpResponse<String> response = client.send(httpRequest, 
            HttpResponse.BodyHandlers.ofString());
        
        if (response.statusCode() == 200) {
            System.out.println("Notification sent successfully: " + response.body());
        } else {
            System.err.println("HTTP error! status: " + response.statusCode() + 
                             ", body: " + response.body());
        }
    }
    
    public static void main(String[] args) {
        try {
            sendNotification();
        } catch (Exception e) {
            System.err.println("Error: " + e.getMessage());
        }
    }
}`
  }

  const handleCopyCommand = (code: string, language: string) => {
    navigator.clipboard
      .writeText(code)
      .then(() => {
        message.success(t`${language} code copied to clipboard!`)
      })
      .catch(() => {
        message.error(t`Failed to copy to clipboard`)
      })
  }

  const CodeBlock: React.FC<{ code: string; language: string }> = ({ code, language }) => (
    <Highlight theme={themes.github} code={code} language={language}>
      {({ className, style, tokens, getLineProps, getTokenProps }) => (
        <pre
          className={className}
          style={{
            ...style,
            fontSize: '12px',
            margin: 0,
            padding: '10px',
            maxHeight: '500px',
            overflow: 'auto'
          }}
        >
          {tokens.map((line, i) => (
            <div key={i} {...getLineProps({ line })}>
              <span
                style={{
                  display: 'inline-block',
                  width: '2em',
                  userSelect: 'none',
                  opacity: 0.3
                }}
              >
                {i + 1}
              </span>
              {line.map((token, key) => (
                <span key={key} {...getTokenProps({ token })} />
              ))}
            </div>
          ))}
        </pre>
      )}
    </Highlight>
  )

  const tabItems = [
    {
      key: 'curl',
      label: 'cURL',
      children: (
        <div>
          <p className="mb-4">
            {t`Use this curl command to send a transactional notification via API:`}
          </p>
          <CodeBlock code={generateCurlCommand()} language="bash" />
        </div>
      )
    },
    {
      key: 'typescript',
      label: 'TypeScript',
      children: (
        <div>
          <p className="mb-4">{t`Send a transactional notification using TypeScript/JavaScript:`}</p>
          <CodeBlock code={generateTypeScriptCode()} language="typescript" />
        </div>
      )
    },
    {
      key: 'python',
      label: 'Python',
      children: (
        <div>
          <p className="mb-4">{t`Send a transactional notification using Python:`}</p>
          <CodeBlock code={generatePythonCode()} language="python" />
        </div>
      )
    },
    {
      key: 'golang',
      label: 'Go',
      children: (
        <div>
          <p className="mb-4">{t`Send a transactional notification using Go:`}</p>
          <CodeBlock code={generateGolangCode()} language="go" />
        </div>
      )
    },
    {
      key: 'java',
      label: 'Java',
      children: (
        <div>
          <p className="mb-4">{t`Send a transactional notification using Java:`}</p>
          <CodeBlock code={generateJavaCode()} language="java" />
        </div>
      )
    },
    ...(window.SMTP_RELAY_ENABLED
      ? [
          {
            key: 'smtp',
            label: 'SMTP',
            children: (
              <div>
                <div className="mb-4">
                  <p className="text-sm">
                    {t`Send transactional notifications using SMTP relay. Perfect for integrating with existing email systems or applications that support SMTP.`}
                  </p>
                </div>
                {renderSMTPInstructions()}
              </div>
            )
          }
        ]
      : [])
  ]

  const [activeTab, setActiveTab] = React.useState('curl')

  const getCurrentCode = () => {
    switch (activeTab) {
      case 'curl':
        return generateCurlCommand()
      case 'typescript':
        return generateTypeScriptCode()
      case 'python':
        return generatePythonCode()
      case 'golang':
        return generateGolangCode()
      case 'java':
        return generateJavaCode()
      case 'smtp':
        return generateSMTPPayload()
      default:
        return generateCurlCommand()
    }
  }

  const getCurrentLanguage = () => {
    switch (activeTab) {
      case 'curl':
        return 'cURL'
      case 'typescript':
        return 'TypeScript'
      case 'python':
        return 'Python'
      case 'golang':
        return 'Go'
      case 'java':
        return 'Java'
      case 'smtp':
        return 'JSON Payload'
      default:
        return 'cURL'
    }
  }

  return (
    <Modal
      title={t`API Command`}
      open={open}
      onCancel={onClose}
      footer={[
        <Button
          key="copy"
          type="primary"
          ghost
          icon={<FontAwesomeIcon icon={faCopy} />}
          onClick={() => handleCopyCommand(getCurrentCode(), getCurrentLanguage())}
        >
          {t`Copy ${getCurrentLanguage()} Code`}
        </Button>,
        <Button key="close" onClick={onClose}>
          {t`Close`}
        </Button>
      ]}
      width={900}
    >
      {notification && (
        <div>
          <Alert
            type="info"
            message={
              <div>
                <div>
                  {t`If the contact email doesn't exist in your workspace, it will be automatically created.`}
                </div>
                <div>
                  {t`Use`} <code>external_id</code> {t`for deduplication - notifications with the same external_id won't be sent twice.`}
                </div>
                <div>{t`All contact fields are optional except email.`}</div>
                <div>
                  {t`Use`} <code>data</code> {t`for template variables,`} <code>metadata</code> {t`for tracking (not available in templates).`}
                </div>
              </div>
            }
            className="!mb-4"
          />

          <Tabs activeKey={activeTab} onChange={setActiveTab} items={tabItems} size="small" />
        </div>
      )}
    </Modal>
  )
}
