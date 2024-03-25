// A Javascript file is used instead of JSON so that environment variables can be pulled in
// via `process.env.VARIABLE_NAME` if needed. This allows secrets to be stored in Github
// then provided to the Renovate config here.
module.exports = {
    $schema: "https://docs.renovatebot.com/renovate-schema.json",
    // This file is empty for now, but it may be useful to be able to add private registry
    // authentication (i.e. ECR) or secrets at a later point.
};
