package services

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

type EmailService struct {
	DB *database.MongoDB
}

func (s *EmailService) loadSettings() (*models.AppSettings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var settings models.AppSettings
	if err := s.DB.Settings().FindOne(ctx, bson.M{}).Decode(&settings); err != nil {
		return nil, fmt.Errorf("failed to load settings: %w", err)
	}
	return &settings, nil
}

func (s *EmailService) SendInviteEmail(toEmail, teamName, inviteURL string) error {
	settings, err := s.loadSettings()
	if err != nil {
		return err
	}
	if settings.MailgunAPIKey == "" || settings.MailgunDomain == "" {
		return fmt.Errorf("Mailgun is not configured")
	}

	from := settings.MailgunFromEmail
	if from == "" {
		from = "noreply@" + settings.MailgunDomain
	}

	subject := fmt.Sprintf("You've been invited to join %s on SocialPod", teamName)
	textBody := fmt.Sprintf(
		"You have been invited to join the team \"%s\" on SocialPod.\n\n"+
			"Click the link below to create your account and join the team:\n\n%s\n\n"+
			"This invitation expires in 7 days.\n\n"+
			"If you did not expect this invitation, you can safely ignore this email.",
		teamName, inviteURL,
	)
	htmlBody := fmt.Sprintf(
		`<div style="font-family: sans-serif; max-width: 600px; margin: 0 auto;">
<h2>Team Invitation</h2>
<p>You have been invited to join the team <strong>%s</strong> on SocialPod.</p>
<p><a href="%s" style="display: inline-block; padding: 12px 24px; background-color: #3b82f6; color: #fff; text-decoration: none; border-radius: 6px; font-weight: 600;">Accept Invitation</a></p>
<p style="color: #666; font-size: 14px;">This invitation expires in 7 days.</p>
<p style="color: #999; font-size: 12px;">If you did not expect this invitation, you can safely ignore this email.</p>
</div>`,
		teamName, inviteURL,
	)

	baseURL := settings.MailgunBaseURL
	if baseURL == "" {
		baseURL = "https://api.mailgun.net"
	}

	return s.sendMailgun(baseURL, settings.MailgunAPIKey, settings.MailgunDomain, from, toEmail, subject, textBody, htmlBody)
}

func (s *EmailService) sendMailgun(baseURL, apiKey, domain, from, to, subject, text, html string) error {
	apiURL := fmt.Sprintf("%s/v3/%s/messages", strings.TrimRight(baseURL, "/"), domain)

	form := url.Values{}
	form.Set("from", from)
	form.Set("to", to)
	form.Set("subject", subject)
	form.Set("text", text)
	form.Set("html", html)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth("api", apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Mailgun API returned status %d", resp.StatusCode)
	}
	return nil
}
