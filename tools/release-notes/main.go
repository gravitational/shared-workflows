/*
Copyright 2024 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/alecthomas/kingpin/v2"
)

var (
	version   = kingpin.Arg("version", "Version to be released").Required().String()
	changelog = kingpin.Arg("changelog", "Path to CHANGELOG.md").Required().String()
	labels    = kingpin.Flag("labels", "Labels to apply to the end of a release, e.g. security labels").String()
)

func main() {
	kingpin.Parse()

	clFile, err := os.Open(*changelog)
	if err != nil {
		log.Fatal(err)
	}
	defer clFile.Close()

	gen := &releaseNotesGenerator{
		releaseVersion: *version,
		labels:         *labels,
	}

	notes, err := gen.generateReleaseNotes(clFile)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(notes)
}
