---
authors: Fred Heinecke (fred.heinecke@goteleport.com)
state: draft
---

# RFD 1 - Tooling Requirements

## Required approvers

* Engineering: (@r0mant)

## What

A set of tooling and repo requirements that should apply to our CI/CD pipeline tooling. This includes both where the tools should live, how they should be released and maintained, and what documentation they should include.

## Why

As our products grow and CI/CD pipelines become more complex, we have an increased need for internal tools that are primarily used in said pipelines. Rather than "reinvent the wheel" [in](https://github.com/gravitational/teleport/tree/master/build.assets/tooling) [each](https://github.com/gravitational/teleport.e/tree/master/tooling/os-package-repo-tool) [repo](https://github.com/gravitational/cloud/tree/master/scripts), we can centralize and standardize our tooling in a single repo. 

This change is designed to lower the development and maintenance overhead of building and writing tools. When I write small to medium sized tools today, I typically write them as inline shell scripts which have a lot of disadvantages. As a developer, I do this because:
* It is the fastest way to write relatively simple software, ensuring that I can deliver my primary task on time.
* If a tool is written (entirely or in part) in a language that requires ahead of time compilation, such as Go, then anything using the tool needs to download the Go compiler, all dependencies, and compile the tool every time it is ran. This increases effective runtime, and increases the chance of failure as there are more moving parts on every run of the tool.
* I need to find a directory for the tool itself to live. This sounds minor, but in some repos (i.e. `gravitational/teleport.e` up until recently) this means defining a new standard, and adds complexity with respect to repo structure. In other cases, such as `gravitational/teleport`, there are multiple directories where a tool could _potentially_ live and it's not always obvious where it should go.
* I know that dependencies will likely not be updated until there is a pressing business reason to do so. In many cases I can write a shell script without installing additional dependencies as the OS that the script will run on usually has what I need. When writing a separate tool in a language like Go, I need at minimum a compiler targeting a specific language version, CLI arg parsing library, and a logging library. To do anything useful I usually also need other third party libraries, for things like talking with AWS or Github. From a security perspective it is much easier to push the overhead of keeping these dependencies up to date to whoever maintains the underlying OS.

Moving CI/CD tooling primarily to one standardized repo has the following advantages:
* Tools can easily be shared cross-repo without creating a spiderweb of dependencies
* We can spend less effort developing and maintaining internal tools by ensuring that each new tool meets a minimum set of standards
* There is a lower chance of a new tool being written that is functionally identical to a pre-existing tool built by another team
* Changes to tooling can be tested, versioned, and released without coupling releases to product releases (in the case of product repos)
* The "tooling for tooling" (in cases like dependency management, builds, and testing) overhead can be reduced


## Details

### Scope

The intention of the RFD is not start a mass migration of existing tools out of their current repos to this one. Rather, it is intended that future tools should live here, and pre-existing tools should be migrated when there is a compelling reason to spend the engineering effort to do so. As a part of my work on the GHA self-hosted runner EKS clusters I have written several tools that I intend to migrate in the near future:
* A tool/GHA that sets up a Github workflow to talk with Kubernetes resources
* A tool/GHA that reconciles a Flux-managed Kubernetes cluster to a specific state, taking into account Flux's idiosyncrasies
* A GHA that self-hosts Renovate, which can be used in several different repos as we start adopting it
* A tool/GHA that retrieves the names and labels of self-hosted GHA runners
* A tool/GHA that extracts Github App JWT and installation tokens for other actions

Here are some examples of other tools that could be added or moved to this repo at some point:
* The OS package repo tool (written in Go) and associated actions
* A Helm chart for deploying new types of ARC runners
* A Go library for interacting with Github workflows

### Repo structure
Existing files in this repo should not be moved until there is a compelling reason to do so. The new files in this repo should have the following general structure:
```
/
├── .github/
│   ├── PULL_REQUEST_TEMPLATE/
│   ├── renovate/
│   │   ├── labels.json5
│   │   └── ...
│   ├── workflows/
│   │   ├── some-tool-cd.yaml -> ../../tools/some-tool/workflows/cd.yaml
│   │   ├── some-tool-ci.yaml -> ../../tools/some-tool/workflows/ci.yaml
│   │   ├── ...
│   ├── CODEOWNERS
│   ├── dependabot.yml
│   ├── renovate-repo-config.js
│   └── renovate.json5
├── bot/
├── libs/
│   ├── some-library/
│   │   ├── docs/
│   │   ├── workflows/
│   │   │   ├── ci.yaml
│   │   │   └── ...
│   │   ├── .gitignore
│   │   ├── CHANGELOG.md
│   │   ├── README.md
│   │   └── renovate.json5
│   └── ...
├── rfd/
│   ├── 0001-tooling-requirements.md
│   └── ...
├── tools/
│   ├── some-tool/
│   │   ├── docs/
│   │   ├── workflows/
│   │   │   ├── cd.yaml
│   │   │   ├── ci.yaml
│   │   │   └── ...
│   │   ├── .gitignore
│   │   ├── CHANGELOG.md
│   │   ├── README.md
│   │   ├── action.yaml
│   │   └── renovate.json5
│   └── ...
├── LICENSE
├── README.md
└── SECURITY.md
```

There are few things to note here:
* Projects should be separated into `tools` and `libs` directories as appropriate. These directories should contain all source code, CI/CD pipelines, documentation, and dependency management configuration associated with each project. The specific layout of source code within these directories is left up to the project's code owner(s).
* Workflows will live in tool and library directories rather than `.github/workflows` at the root of the repo. This will help ensure that it is clear which workflows correspond with each tool and library. Github still requires that all workflows live in the repo root, so they will be symlinked in `.github/workflows/<project>-<workflow>.yaml` targeting `<lib|tooling>/<project>/workflows/workflow>.yaml` instead.
* While Dependabot has historically been used for keeping this repo's tool's dependencies up to date, Renovate will now be added as well. See the [Dependency management](#dependency-management) section for details on why this choice was made. The structure of the configuration will be similar to `gravitational/cloud-terraform`, with a top-level config and individual configs for each tool. This allows Renovate to be specifically configured for each tool.
* There will be a pull request template for new projects that includes a checklist of all the items listed in [Project requirements](#project-requirements).
* Some additional boilerplate files will be added such as a `LICENSE` and `SECURITY.md` file. The contents of these files will be copy/pasted from `gravitational/teleport` and tweaked as appropriate.

### Project requirements
New "projects" (tools and libraries) should live in dedicated directories under `tools` and `libs`, wherever is most appropriate. Each project directory should be treated as it's own separate repo in that all resources associated with the project's lifecycle should be self-contained in the directory.

Projects should have the following resources in their directory:
* A `README.md` document at the project root. This should detail what the project is and how to run it, including a least one explicit example. This should also contain information on any major nuances that end users should be aware of. For example, a README for the OS package repo tool would document that the tool should never be ran concurrently when targeting the same S3 bucket.
* A `CHANGELOG.md` document that follows the standard [described here](https://keepachangelog.com/).
* A `renovate.json5` file that configures any [Renovate managers](https://docs.renovatebot.com/modules/manager/) required to keep the project's dependencies up to date.
* An `action.yaml` file (optional) for tools that support being ran as Github actions. This action should _not_ compile the tool, download dependencies, or perform any other actions that could reasonably be completed once per release. In the case of non-composite actions, this is intended to be a simple GHA-friendly wrapper of the underlying tool.
* A `.gitignore` file (optional) that contains project-specific filepath patterns that Git should ignore.
* A `docs` directory (optional) which includes additional useful information regarding the project that is not included in the README.
* A `workflows` directory which all Github workflows related to the project's lifecycle. This includes (at minimum) a CI workflow for validating PR changes, a CD workflow for releasing dev tag versions upon PR merge, and a release/promotion workflow for promoting a dev tag release when a new version is added to the changelog. 

#### CI workflow
The status check workflows should not trigger upon changes to specific paths, rather, they should follow the precedent set in the `gravitational/cloud` repo and run on every PR that is opened. Each workflow should then use an action [like this one](https://github.com/dorny/paths-filter) to determine whether or not the check should run, or be bypassed. This removes the need for the superfluous "bypass" workflow pattern used in other repositories in the Gravitational org.

#### CD workflow
This workflow should run when a PR is merged, and optionally upon PR commits as a way to produce test builds. When ran, this workflow should upload artifacts as Github Packages where possible. These artifacts should not be uploaded to AWS unless absolutely necessary, and with Security's permission, as outlined in the [Security considerations section](#security-considerations). The name of these artifacts should include a dev tag generated by `git describe --tags` to ensure uniqueness.

#### Release workflow
The release workflow should be triggered when a new version is added to the CHANGELOG on `main` branch. When this occurs, the workflow should:
1. Extract the new version from the CHANGELOG.
2. Create a new Git tag targeting the commit that modified the CHANGELOG, with name `v<new version>-<project name>` with all separator characters (e.g. spaces and underscores) separated by dashes. This tag should be parsable by [this npm package](https://www.npmjs.com/package/semver) that Renovate uses for semver coercion.
3. Build a new release of the tool, following the general logic of the CD workflow. Either the release version or Git tag may be used where appropriate.
4. Create a new release, named after the Git tag, that includes the changes from the CHANGELOG. The release should also either include the built artifacts, or instructions on retrieving them (such as `docker pull ghcr.io/gravitational/shared-workflows/some-project:v1.2.3`).

### Project releases
The process for releasing a new version of a project should be as follows:
1. PRs are filed to add/modify the project.
2. The project's CI workflow triggers to validate the changes, codeowners approve, and PRs are merged.
3. Artifacts are built for each PR to facilitate release testing.
4. A new PR is filed and merged that updates the changelog to a new version.
5. The release workflow is triggered, which creates a new tag and publishes a fresh build.

### Dependency management
Historically this repo has used Dependabot to manage dependency updates. Unfortunately however, Dependabot does not support two key "languages" used by Github Actions: actions themselves (used when building composite actions), and Dockerfiles (used when building Dockerfile actions). As a result, [Renovate](https://docs.renovatebot.com/) will be used for new and migrated projects instead. Renovate is highly flexible and can be configured on a per-project basis.

Renovate will self-hosted and configured to run daily. Updates for each project should be grouped as much as reasonably possible to reduce the number of PRs opened at any given time. Update PRs will be assigned to code owners for each project. Digest pinning will be used wherever supported.

### Security considerations
The primary source of security concern with anything living in this repo is that the repo is public. The impact of making a mistake with one of our workflows (such as exposing a sensitive value) is much greater than if the repo was private/internal.

I took a look at the forks, stars, watchers, issues, and PRs for this repo. As far as I can tell, all but two or three of the interactions (stars and watchers) from non-Gravitational Github accounts are from bots. Based on this I think it is reasonable to assume that nobody outside of Gravitational uses this repo. Additionally, I don't personally believe that the few existing tools in this repo have a large value to the open source community. With this information in mind, I believe that we make this repo private. Doing so would alleviate most of the security concerns associated with this RFD.

If we choose not to go this route then we need to be very careful about the workflows we write for this repository, the secrets we make available, and the AWS access that this repo can use. Additionally, we need to thoroughly vet our self-hosted GHA runners before we can consider using them for workflows here.