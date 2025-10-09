# mac-packaging

## Usage

### Packaging

App Bundle (.app)
```shell
telebuild package-app tsh tsh.app/
```

Package Installer (.pkg)

```shell
# Staging files
mkdir "${STAGING_PKG}"
cp "file1" "file2" "${STAGING_PKG}"

# Package
telebuild package-pkg --install-location /usr/local/bin "${STAGING_PKG}" "my-app.pkg"
```

### Notarization

By default, notarization is disabled and will output dryrun logs. To enable it you must either set the following options:
```shell
telebuild --apple-username="" --apple-password="" --signing-identity="" --bundle-id="" ...
```

These flags can also be set through the environment.
```shell
APPLE_USERNAME=""
APPLE_PASSWORD=""
SIGNING_IDENTITY=""
BUNDLE_ID=""
```

If all of these are set then notarization will be enabled and the tool will notarize after packaging.
This is to make it convenient to test locally without having to set up creds to build packages.

However this isn't desirable in CI environments where notarization must happen. Enabling dryrun will "silently" cause a failure.
For convenience the `--force-notarization` flag is provided to fail in the scenario where creds are missing.
