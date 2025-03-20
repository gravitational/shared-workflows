* Add audit event field describing if the "MFA for admin actions" requirement changed.
* Display errors in the web UI console for SSH and Kubernetes sessions.
* Update go-retryablehttp to v0.7.7 (fixes CVE-2024-6104).
* Fixes an issue preventing accurate inventory reporting of the updater after it is removed.
* Fix input searching logic for Connect's access request listing view.
* Debug setting available for event-handler via configuration or FDFWD_DEBUG environment variable.
* Fix Headless auth for sso users, including when local auth is disabled.
* Add configuration for custom CAs in the event-handler helm chart.
* VNet panel in Teleport Connect now lists custom DNS zones and DNS zones from leaf clusters.
* Fixed an issue with Database Access Controls preventing users from making additional database connections depending on their permissions.
* Fixed bug that caused gRPC connections to be disconnected when their certificate expired even though DisconnectCertExpiry was false.
* Fixed Connect My Computer in Teleport Connect failing with "bind: invalid argument".
* Fix a bug where a Teleport instance running only Jamf or Discovery service would never have a healthy  `/readyz` endpoint.
* Added a missing `[Install]` section to the `teleport-acm` systemd unit file as used by Teleport AMIs.
* Patched timing variability in curve25519-dalek.
* Fix setting request reason for automatic ssh access requests.
* Improved log rotation logic in Teleport Connect; now the non-numbered files always contain recent logs.
* Adds `tctl desktop bootstrap` for bootstrapping AD environments to work with Desktop Access.
