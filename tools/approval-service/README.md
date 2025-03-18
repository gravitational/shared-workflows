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

### GitHub App

A GitHub App must be created and installed to the repositories you want the tool to manage.
This will give us credentials necessary to authenticate to approve deployment reviews and also allows 
us to subscribe to `deployment_review` events which are not normally subscribable from the normal webhook configuration.

**Repository Permissions**:

* Deployments: Read and write

**Subscribe to events**:

* Deployment review: Deployment review requested, approved or rejected 

## Security
TODO
