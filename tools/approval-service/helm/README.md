# approval-service-chart

## Configuration

The following shows the recommended configuration for the chart.

```yaml
image:
  # repository is docker repository to pull the container image from
  repository: 00000000000.dkr.ecr.us-west-2.amazonaws.com/approval-service
  # tag represents the version of the approval service
  tag: "v0.0.0"
# tbot configuration is to setup authentication for the Machine ID
# It is by default setup to use IAM in the chart which can be configured in Teleport.
tbot:
  clusterName: platform-staging.teleport.sh
  teleportProxyAddress: "platform-staging.teleport.sh:443"
  annotations:
    serviceAccount:
      eks.amazonaws.com/role-arn: "arn:aws:iam::966006926981:role/approval-service-bot-join"
ingress:
  # enabled controls whether to create an ingress resource for external access
  enabled: true
  # hostname is the domain name where the approval service will be accessible
  hostname: approval-service.teleport.dev
appConfig:
  approval_service:
    teleport:
      # proxy_addrs is a list of Teleport proxy addresses to connect to
      proxy_addrs:
      - platform-staging.teleport.sh:443
      # user is the Teleport bot user that the approval service will authenticate as
      user: bot-approval-service
    # request_ttl_hours is how long approval requests remain valid (in hours)
    request_ttl_hours: 168
    # listen_addr is the address and port the approval service HTTP server binds to
    listen_addr: 0.0.0.0:8080
  event_sources:
    github:
      # path is the webhook endpoint path that GitHub will POST to
      path: /
      # org is the GitHub organization to monitor for events
      org: gravitational
      # repo is the specific repository to monitor within the organization
      repo: teleport
      # environments define the deployment environments and their associated Teleport roles
      environments:
      - name: prod
        # teleport_role is the Teleport role that will be granted for this environment
        teleport_role: prod-role
      - name: staging
        teleport_role: staging-role

appSecrets:
  # webhookSecret is used to verify that webhook requests are from GitHub
  webhookSecret: "webhook-secret"
  # appId is the GitHub App ID (integer)
  appId: 1234567
  # installationId is the GitHub App installation ID (integer)
  installationId: 765544312
  # privateKey is the base64-encoded private key for the GitHub App
  privateKey: "base64-encoded-private-key"
```

To see all configuration options you can look at `values.yaml` but special care is needed for the extra options.
