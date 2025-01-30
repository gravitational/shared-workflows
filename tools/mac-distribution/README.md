# mac-packaging

## Usage

Notarize Binaries

```
APPLE_USERNAME=""
APPLE_PASSWORD=""
APPLE_DEVELOPER_TEAM_ID=""


mkdir ${STAGING_DIR}
ditto ${BINARIES} ${STAGING_DIR}
mac-distribution notarize-binaries --retry 2 --force-notarization ${STAGING_DIR}
```

Notarize App Bundle

```
mac-distribution app-bundle \
    --name tsh \
    --version $(VERSION) \
    --bundle $(TSH_APP_BUNDLE) \
    --entitlements $(TSH_APP_ENTITLEMENTS) \
    --app-binary $(BUILDDIR)/tsh
```

