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
  enabled: true
  hostname: approval-service.teleport.dev
appConfig:
  approval_service:
    teleport:
      proxy_addrs:
      - platform-staging.teleport.sh:443
      user: bot-approval-service
    request_ttl_hours: 168
    listen_addr: 0.0.0.0:8080
  event_sources:
    github:
      path: /
      org: gravitational
      repo: teleport
      environments:
      - name: prod
        teleport_role: prod-role
      - name: staging
        teleport_role: staging-role

appSecrets:
  webhookSecret: "webhook-secret"
  appId: 1234567
  installationId: 765544312
  privateKey: "base64-encoded-private-key"
```

To see all configuration options you can look at `values.yaml` but special care is needed for the extra options.
