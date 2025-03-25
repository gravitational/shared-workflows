# approval-service

This service approves or denies CI/CD pipeline jobs. Approvals/denials are
handled by Teleport.

The following CI/CD tools are currently supported:
* GitHub Actions
    * Provides workflow dispatch event information, and "rolls up" requests for
      multiple workflows jobs into a single approval/denial.
    * Supports automated denial for requests from outside the GitHub
      organization.

## Prerequisites

These are steps that are done before 

### GitHub App

A GitHub App must be created and installed to the repositories you want the tool to manage.
This will give us credentials necessary to authenticate to approve deployment reviews and also allows 
us to subscribe to `deployment_review` events which are not normally subscribable from the normal webhook configuration.

**Repository Permissions**:

* Deployments: Read and write

**Subscribe to events**:

* Deployment review: Deployment review requested, approved or rejected.

### Teleport

**Role**:

Two roles must be created for the Pipeline Approval Service:

* Environment Access Role
  * The PAS will create an Access Request to this role to indicate access to a GitHub Environment.
  * Needs no permissions.
* Bot Role
  * Required to create a bot.
  * Needs no permissions.

**Bot**:

A bot needs to be created for the PAS.
As stated above, a role needs to be created for the bot but it does not need permissions.
Instead after the bot is created a default role is also created with it.
This default role needs to be manually updated with the following permissions:

```yaml
allow:
  access:
    roles:
    - <environment-access-role>
  rules:
  - resources:
    - access_request
    verbs:
    - "*"
```

## Deployment information
TODO

## Security
TODO
