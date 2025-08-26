/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/certprovider"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/certprovider/pemfile"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/certprovider/teleportworkloadidentity"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/gpg/archive"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/gpg/keyid"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	localfilemanager "github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager/local"
	s3filemanager "github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager/s3"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/packagemanager"
	aptpkg "github.com/gravitational/shared-workflows/tools/oprt2/pkg/packagemanager/apt"
)

// Instantiate business types for each config type.

func GetLogger(config *Logger) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(config.Level)); err != nil {
		return nil, fmt.Errorf("failed to set log level: %w", err)
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	if level <= slog.LevelDebug {
		opts.AddSource = true
	}

	var handler slog.Handler
	switch config.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler), nil
}

func GetAttuneAuthenticator(config Authenticator) (commandrunner.Hook, error) {
	switch {
	case config.MTLS != nil:
		return GetMTLSAuthenticator(config.MTLS)
	case config.Token != nil:
		return NewTokenAuthenticator(config.Token)
	default:
		return nil, fmt.Errorf("no or unknown Attune authenticator specified")
	}
}

func GetMTLSAuthenticator(config *MTLSAuthenticator) (commandrunner.Hook, error) {
	certProvider, err := GetCertificateSource(config.CertificateSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get mTLS certificate source: %w", err)
	}

	hook, err := authenticators.NewMutualTLSAuthenticator(config.Endpoint, certProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create mTLS authenticator: %w", err)
	}

	return hook, nil
}

func GetCertificateSource(config CertificateProvider) (certprovider.Provider, error) {
	switch {
	case config.PEMFile != nil:
		return NewPEMFileProvider(config.PEMFile), nil
	case config.Teleport != nil:
		return GetTeleportProvider(config.Teleport)
	default:
		return nil, fmt.Errorf("no certificate source provided")
	}
}

func NewPEMFileProvider(config *PEMFileCertificateProvider) *pemfile.PEMFileProvider {
	return pemfile.NewPEMProvider(config.PrivateKey, config.PublicKey, pemfile.WithCertChain(config.KeychainKeys...))
}

func GetTeleportProvider(config *TeleportCertificateProvider) (certprovider.Provider, error) {
	switch {
	case config.WorkloadIdentity != nil:
		return NewTeleportWorkloadIdentityProvider(config.WorkloadIdentity), nil
	default:
		return nil, fmt.Errorf("no Teleport certificate source provided")
	}
}

func NewTeleportWorkloadIdentityProvider(config *TeleportWorkloadIdentityCertificateProvider) *teleportworkloadidentity.TeleportWorkloadIdentityProvider {
	return teleportworkloadidentity.NewTeleportWorkloadIdentityProvider(config.Name, teleportworkloadidentity.WithTTL(config.TTL))
}

func NewTokenAuthenticator(config *TokenAuthenticator) (*authenticators.TokenAuthenticator, error) {
	hook, err := authenticators.NewTokenAuthenticator(config.Endpoint, config.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to build token authenticator: %w", err)
	}

	return hook, nil
}

func GetPackageManagers(ctx context.Context, configs []PackageManager, attuneAuthHooks ...commandrunner.Hook) ([]packagemanager.PackageManager, []packagemanager.ClosablePackageManager, error) {
	packageManagers := make([]packagemanager.PackageManager, 0, len(configs))
	closableManagers := make([]packagemanager.ClosablePackageManager, 0, len(configs))

	for i, config := range configs {
		packageManager, err := GetPackageManager(ctx, config, attuneAuthHooks...)
		if err != nil {
			return nil, closableManagers, fmt.Errorf("failed to get package manager %d: %w", i+1, err)
		}

		if closableManager, ok := packageManager.(packagemanager.ClosablePackageManager); ok {
			closableManagers = append(closableManagers, closableManager)
		}

		packageManagers = append(packageManagers, packageManager)
	}

	return packageManagers, closableManagers, nil
}

func GetPackageManager(ctx context.Context, config PackageManager, attuneAuthHooks ...commandrunner.Hook) (packagemanager.PackageManager, error) {
	switch {
	case config.APT != nil:
		return NewAPT(ctx, config.APT, attuneAuthHooks...)
	default:
		return nil, fmt.Errorf("no package manager config provided")
	}
}

func NewAPT(ctx context.Context, config *APT, attuneAuthHooks ...commandrunner.Hook) (*aptpkg.APT, error) {
	fm, err := GetFileManager(ctx, config.FileSource)
	if err != nil {
		return nil, fmt.Errorf("failed to build file manager for APT source files: %w", err)
	}

	components, err := GetAPTComponents(config.Components)
	if err != nil {
		return nil, fmt.Errorf("failed to get APT components: %w", err)
	}

	// This must done last to avoid leaking the provider if something else fails
	gpgProvider := GetGPGProvider(config.GPG)

	apt := aptpkg.NewAPT(
		fm,
		aptpkg.WithAttuneHooks(append(attuneAuthHooks, gpgProvider)...),
		aptpkg.WithComponents(components),
		aptpkg.WithDistros(config.Distros),
	)
	return apt, nil
}

func GetAPTComponents(config map[string][]string) (map[string][]*regexp.Regexp, error) {
	components := make(map[string][]*regexp.Regexp, len(config))
	for componentName, fileMatchers := range config {
		fileMatcherExpressions := make([]*regexp.Regexp, 0, len(fileMatchers))
		for _, fileMatcher := range fileMatchers {
			fileMatcherExpression, err := regexp.Compile(fileMatcher)
			if err != nil {
				return nil, fmt.Errorf("failed to parse file matcher %q for APT component %q: %w", fileMatcher, componentName, err)
			}

			fileMatcherExpressions = append(fileMatcherExpressions, fileMatcherExpression)
		}

		components[componentName] = fileMatcherExpressions
	}

	return components, nil
}

func GetFileManager(ctx context.Context, config FileManager) (filemanager.FileManager, error) {
	switch {
	case config.Local != nil:
		return NewLocalFileManager(config.Local)
	case config.S3 != nil:
		return NewS3FileManager(ctx, config.S3)
	default:
		return nil, fmt.Errorf("no file manager specified")
	}
}

func NewLocalFileManager(config *LocalFileManager) (*localfilemanager.LocalFileManager, error) {
	fm, err := localfilemanager.NewLocalFileManager(config.Directory)
	if err != nil {
		return nil, fmt.Errorf("failed to create new local file managere: %w", err)
	}

	return fm, nil
}

func NewS3FileManager(ctx context.Context, config *S3FileManager) (*s3filemanager.S3FileManager, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config from default sources: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	sfm, err := s3filemanager.NewS3FileManager(client, config.Bucket, s3filemanager.WithPathPrefix(config.Prefix))
	if err != nil {
		return nil, fmt.Errorf("failed to build S3 file manager: %w", err)
	}

	return sfm, nil
}

func GetGPGProvider(config *GPGProvider) commandrunner.Hook {
	switch {
	case config.Archive != nil:
		return NewGPGArchiveProviderHook(config.Archive)
	case config.KeyID != nil:
		return NewGPGKeyIDProviderHook(config.KeyID)
	}

	return nil
}

func NewGPGArchiveProviderHook(config *GPGArchiveProvider) commandrunner.Hook {
	opts := make([]archive.ArchiveProviderOption, 0, 1)

	if config.KeyID != nil {
		opts = append(opts, archive.WithKeyID(*config.KeyID))
	}

	return archive.NewArchiveProvider(config.Archive, opts...)
}

func NewGPGKeyIDProviderHook(config *GPGKeyIDProvider) commandrunner.Hook {
	opts := make([]keyid.KeyIDProviderOption, 0, 2)
	if config.GPGHomeDirectory != nil {
		opts = append(opts, keyid.WithGPGHomeDir(*config.GPGHomeDirectory))
	}

	if config.KeyID != nil {
		opts = append(opts, keyid.WithKeyID(*config.KeyID))
	}

	return keyid.NewKeyIDProvider(opts...)
}
