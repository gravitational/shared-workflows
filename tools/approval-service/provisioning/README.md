## provisioning

This contains Terraform configuration for provisioning pre-requisite resources for the approval service.
These resources are outside the normal software development lifecycle of the service and are in a separate module to indicate that.

* Teleport Bots & Roles
* ECR Repository

## Manual Steps

Due to some issues with the current implementation of bots, we must manually update the default bot role to grant permissions to create Access Requests.
