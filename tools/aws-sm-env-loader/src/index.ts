import * as core from "@actions/core";
// import { SecretsManagerClient } from "@aws-sdk/client-secrets-manager";

async function run(): Promise<void> {
  /*
  1. Get the name of the current environment
  2. Look up role, secret based upon environment name 
    1. Input JSON
    2. JSON file
      1. Current path, look upwards
      2. Load from GH (needed if repo not cloned)
  3. Get the secret version to load
    1. Input field
    2. Branch name
    3. "LATEST" and warn
  4. Load the secret
  5. Load variables and secrets into output context
    1. Mask secrets
    2. If configured, set vars/secrets as env vars
  */

  core.info("test");

  // const inputs = core.getInputs();
  // const version = versionString(os.platform(), os.arch(), inputs.version);
  // const toolName = inputs.enterprise ? 'teleport-ent' : 'teleport';
  // core.info(`Installing ${toolName} ${version}`);

  // const toolPath = tc.find(toolName, version);
  // if (toolPath !== '') {
  //     core.info('Teleport binaries found in cache.');
  //     core.addPath(toolPath);
  //     return;
  // }

  // core.info('Could not find Teleport binaries in cache. Fetching...');
  // core.debug('Downloading tar');
  // const downloadPath = await tc.downloadTool(
  //     `https://cdn.teleport.dev/${toolName}-${version}-bin.tar.gz`
  // );

  // core.debug('Extracting tar');
  // const extractedPath = await tc.extractTar(downloadPath, undefined, [
  //     'xz',
  //     '--strip',
  //     '1',
  // ]);

  // core.info('Fetched binaries from Teleport. Writing them back to cache...');
  // const cachedPath = await tc.cacheDir(extractedPath, toolName, version);
  // core.addPath(cachedPath);
}

run().catch(core.setFailed);
