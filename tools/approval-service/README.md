# approval-service

This service approves or denies CI/CD pipeline jobs. Approvals/denials are
handled by Teleport.

The following CI/CD tools are currently supported:
* GitHub Actions
    * Provides workflow dispatch event information, and "rolls up" requests for
      multiple workflows jobs into a single approval/denial.
    * Supports automated denial for requests from outside the GitHub
      organization.

## Deployment information
TODO

## Security
TODO
