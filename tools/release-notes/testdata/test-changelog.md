# Changelog

## 17.1.3 (1/2/25)

* Fixes a bug where v16 Teleport cannot connect to v17.1.0, v17.1.1 and v17.1.2 clusters. [#50658](https://github.com/gravitational/teleport/pull/50658)
* Prevent panicking during shutdown when SQS consumer is disabled. [#50648](https://github.com/gravitational/teleport/pull/50648)
* Add a --labels flag to the tctl tokens ls command. [#50624](https://github.com/gravitational/teleport/pull/50624)

## 16.0.1 (06/17/24)

* `tctl` now ignores any configuration file if the auth_service section is disabled, and prefer loading credentials from a given identity file or tsh profile instead. [#43115](https://github.com/gravitational/teleport/pull/43115)
* Skip `jamf_service` validation when the service is not enabled. [#43095](https://github.com/gravitational/teleport/pull/43095)
* Fix v16.0.0 amd64 Teleport plugin images using arm64 binaries. [#43084](https://github.com/gravitational/teleport/pull/43084)
* Add ability to edit user traits from the Web UI. [#43067](https://github.com/gravitational/teleport/pull/43067)
* Enforce limits when reading events from Firestore for large time windows to prevent OOM events. [#42966](https://github.com/gravitational/teleport/pull/42966)
* Allow all authenticated users to read the cluster `vnet_config`. [#42957](https://github.com/gravitational/teleport/pull/42957)
* Improve search and predicate/label based dialing performance in large clusters under very high load. [#42943](https://github.com/gravitational/teleport/pull/42943)

## 16.0.0 (06/13/24)

Teleport 16 brings the following new features and improvements:

- Teleport VNet
- Device Trust for the Web UI
- Increased support for per-session MFA
- Web UI notification system
- Access requests from the resources view
- `tctl` for Windows
- Teleport plugins improvements

## 15.4.17 (08/28/24)

* Prevent connections from being randomly terminated by Teleport proxies when `proxy_protocol` is enabled and TLS is terminated before Teleport Proxy. [#45993](https://github.com/gravitational/teleport/pull/45993)
* Fixed an issue where host_sudoers could be written to Teleport proxy server sudoer lists in Teleport v14 and v15. [#45961](https://github.com/gravitational/teleport/pull/45961)
* Prevent interactive sessions from hanging on exit. [#45953](https://github.com/gravitational/teleport/pull/45953)
* Fixed kernel version check of Enhanced Session Recording for distributions with backported BPF. [#45942](https://github.com/gravitational/teleport/pull/45942)
* Added a flag to skip a relogin attempt when using `tsh ssh` and `tsh proxy ssh`. [#45930](https://github.com/gravitational/teleport/pull/45930)
* Fixed an issue WebSocket upgrade fails with MiTM proxies that can remask payloads. [#45900](https://github.com/gravitational/teleport/pull/45900)
* When a database is created manually (without auto-discovery) the teleport.dev/db-admin and teleport.dev/db-admin-default-database labels are no longer ignored and can be used to configure database auto-user provisioning. [#45892](https://github.com/gravitational/teleport/pull/45892)
* Slack plugin now lists logins permitted by requested roles. [#45854](https://github.com/gravitational/teleport/pull/45854)
* Fixed an issue that prevented the creation of AWS App Access for an Integration that used digits only (eg, AWS Account ID). [#45818](https://github.com/gravitational/teleport/pull/45818)
* For new EKS Cluster auto-enroll configurations, the temporary Access Entry is tagged with `teleport.dev/` namespaced tags. For existing set ups, please add the `eks:TagResource` action to the Integration IAM Role to get the same behavior. [#45726](https://github.com/gravitational/teleport/pull/45726)
* Added support for importing S3 Bucket Tags into Teleport Policy's Access Graph. For existing configurations, ensure that the `s3:GetBucketTagging` permission is manually included in the Teleport Access Graph integration role. [#45550](https://github.com/gravitational/teleport/pull/45550)
