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

import "time"

// Struct tags are needed for three reasons:
// * They allow for generation of JSON schema files
// * They allow for parsing of json config files
// * They allow for validation of parsed config files
//
// When parsing, the YAML config file is converted to JSON. YAML is easier for
// humans to read and write, but most validation and schema libraries operate
// on JSON.

//go:generate go run ./../../cmd/schemagen/ ./../../schema/config.json

type TeleportWorkloadIdentityCertificateProvider struct {
	Name string        `json:"name" jsonschema:"required"`
	TTL  time.Duration `json:"ttl" jsonschema:"default=15m"`
}

type TeleportCertificateProvider struct {
	WorkloadIdentity *TeleportWorkloadIdentityCertificateProvider `json:"workloadIdentity" jsonschema:"required"`
	// Not implemented: certs managed by `tsh app login`
}

type PEMFileCertificateProvider struct {
	PrivateKey   string   `json:"privateKey" jsonschema:"required"`
	PublicKey    string   `json:"publicKey" jsonschema:"required"`
	KeychainKeys []string `json:"keychainKeys"`
}

type CertificateProvider struct {
	PEMFile  *PEMFileCertificateProvider  `json:"pemFile"`
	Teleport *TeleportCertificateProvider `json:"teleport" jsonschema:"minProperties=1,maxProperties=1"`
}

type MTLSAuthenticator struct {
	Endpoint          string              `json:"endpoint" jsonschema:"required"`
	CertificateSource CertificateProvider `json:"certificateSource" jsonschema:"required,minProperties=1,maxProperties=1"`
}

type TokenAuthenticator struct {
	Endpoint string `json:"endpoint" jsonschema:"required"`
	Token    string `json:"token" jsonschema:"required"`
}

type Authenticator struct {
	Token *TokenAuthenticator `json:"token"`
	MTLS  *MTLSAuthenticator  `json:"mTLS"`
	// Not implemented: authentication handled via `tsh proxy app`
}

type S3FileManager struct {
	Bucket string `json:"bucket" jsonschema:"required"`
	Prefix string `json:"prefix"`
}

type LocalFileManager struct {
	Directory string `json:"directory" jsonschema:"required"`
}

type FileManager struct {
	Local *LocalFileManager `json:"local"`
	S3    *S3FileManager    `json:"s3"`
}

type GPGArchiveProvider struct {
	Archive string  `json:"archive" jsonschema:"required"`
	KeyID   *string `json:"keyID"`
}

type GPGKeyIDProvider struct {
	KeyID            *string `json:"keyID"`
	GPGHomeDirectory *string `json:"gpgHomeDirectory"`
}

type GPGProvider struct {
	Archive *GPGArchiveProvider `json:"archive"`
	KeyID   *GPGKeyIDProvider   `json:"keyID"`
}

type APT struct {
	FileSource FileManager         `json:"fileSource" jsonschema:"required,minProperties=1,maxProperties=1"`
	GPG        *GPGProvider        `json:"gpg" jsonschema:"minProperties=1,maxProperties=1"`
	Components map[string][]string `json:"components" jsonschema:"required,minProperties=1"`
	Distros    map[string][]string `json:"distros" jsonschema:"required,minProperties=1"`
}

type PackageManager struct {
	APT *APT `json:"apt"`
}

type Attune struct {
	Authentication      Authenticator `json:"authentication" jsonschema:"required,minProperties=1,maxProperties=1"`
	ParallelUploadLimit uint          `json:"parallelUploadLimit" jsonschema:"default=20"`
}

type Logger struct {
	Level  string `json:"level" jsonschema:"enum=debug info warn error,default=info"`
	Format string `json:"format" jsonschema:"enum=text json,default=text"`
}

type OPRT2 struct {
	Logger          *Logger          `json:"logger"`
	Attune          Attune           `json:"attune" jsonschema:"required"`
	PackageManagers []PackageManager `json:"packageManagers" jsonschema:"minProperties=1,maxProperties=1"`
}
