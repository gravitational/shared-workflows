package internal

// Config is the configuration for the approval service.
type Config struct {
	// GitHubWebhook is the configuration for the GitHub webhook.
	GitHubWebhook GitHubWebhookConfig
}

type GitHubWebhookConfig struct {
	// Address is the address to listen for GitHub webhooks.
	Address string
	// Secret is the secret used to authenticate the webhook.
	Secret string
}
