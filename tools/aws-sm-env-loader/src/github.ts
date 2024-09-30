// import * as core from "@actions/core";
import { Octokit } from "@octokit/core";
import { context } from "@actions/github";
import {
  PageInfoBackward,
  paginateGraphQL,
} from "@octokit/plugin-paginate-graphql";

// General approach pulled from
// https://github.com/qoomon/actions--context/blob/f44b5b5848f4c233f8e810e7e359bdc475d06245/lib/actions.ts#L269
// but was entirely written from scratch with significant improvements.

// Parse the $ACTIONS_ID_TOKEN_REQUEST_URL variable to get the "external ID"
// for the currently running job. This can be used via an API call to get the
// normal "job ID" used in API calls pretty much everywhere else. This requires
// `id-token: write` permissions, but is the only way to get this information.
function getJobExternalId(): string {
  const tokenUrl = process.env["ACTIONS_ID_TOKEN_REQUEST_URL"];
  if (tokenUrl === undefined) {
    throwPermissionError(
      { scope: "id-token", permission: "write" },
      { cause: "ACTIONS_ID_TOKEN_REQUEST_URL env var is not set" }
    );
  }

  const tokenUrlMatch = tokenUrl.match(/jobs\/(?<job_external_id>.+)\//);
  if (tokenUrlMatch === null) {
    throw new Error(`Token URL "${tokenUrl}" did not match regex`);
  }

  return tokenUrlMatch.groups?.job_external_id as string;
}

type WorkflowRun = {
  checkSuite: {
    id: string;
  };
};

let _workflowRun: ReturnType<typeof getWorkflowRun>;

/**
 * Gets details about the current workflow run. Note that a run encompasses
 * all _jobs_, and is not the job itself
 * @param octokit - API client to use
 */
async function getWorkflowRun(
  octokit: InstanceType<typeof Octokit>
): Promise<WorkflowRun> {
  if (_workflowRun !== null) return _workflowRun;

  type WorkflowRunResponse = {
    resource: WorkflowRun;
  };

  return (_workflowRun = octokit
    .graphql<WorkflowRunResponse>(
      `query run($run_url:URI!){
      resource(url:$run_url) {
        ... on WorkflowRun{
          checkSuite{
            id
          }
        }
      }
    }`,
      {
        run_url: `${context.serverUrl}/${context.repo}/actions/runs/${context.runId}`,
      }
    )
    .then((response) => response.resource));
}

type WorkflowJob = {
  databaseId: string;
  externalId: string;
};

let _workflowJobs: ReturnType<typeof getWorkflowJobs>;

/**
 * Gets details about all the workflow jobs for the currently executing run.
 * @param octokit - API client to use
 */
async function getWorkflowJobs(
  octokit: InstanceType<typeof Octokit>
): Promise<WorkflowJob[]> {
  if (_workflowJobs !== null) return _workflowJobs;

  const workflowRun = getWorkflowRun(octokit);

  type WorkflowJobResponse = {
    node: {
      checkRuns: {
        nodes: WorkflowJob[];
        pageInfo: PageInfoBackward;
      };
    };
  };

  const paginatedOctokit = paginateGraphQL(octokit);
  return (_workflowJobs = paginatedOctokit.graphql
    .paginate<WorkflowJobResponse>(
      `query cs($cs_id:ID!, $cursor: String){
        node(id:$cs_id) {
          ... on CheckSuite {
            checkRuns(last: 100, after: $cursor) {
              nodes {
                databaseId
                externalId
              }
              pageInfo {
                startCursor
                hasPreviousPage
              }
            }
          }
        }
      }`,
      {
        cs_id: (await workflowRun).checkSuite.id,
      }
    )
    .then((response) => response.node.checkRuns.nodes));
}

let _currentWorkflowJob: ReturnType<typeof getCurrentWorkflowJob>;

/**
 * Gets the workflow job that executes this code.
 * @param octokit - API client to use
 */
async function getCurrentWorkflowJob(
  octokit: InstanceType<typeof Octokit>
): Promise<WorkflowJob> {
  if (_currentWorkflowJob !== null) return _currentWorkflowJob;

  const jobExternalId = getJobExternalId();

  return (_currentWorkflowJob = getWorkflowJobs(octokit).then((jobs) => {
    const currentJob = jobs.find((job) => job.externalId === jobExternalId);
    if (currentJob === undefined) {
      throw new Error("Unable to find currently running job via API");
    }

    return currentJob;
  }));
}

type Deployment = {
  environment: string;
  state: string;
  statuses: {
    nodes: {
      environment: string;
      logUrl: string;
      state: string;
    }[];
  };
};

let _currentDeployment: ReturnType<typeof getCurrentDeployment>;

export async function getCurrentDeployment(
  octokit: InstanceType<typeof Octokit>
): Promise<Deployment> {
  if (_currentDeployment !== null) return _currentDeployment;

  const currentWorkflowJob = await getCurrentWorkflowJob(octokit);

  type DeploymentQueryResponseType = {
    repository: {
      object: {
        deployments: {
          nodes: Deployment[];
          pageInfo: PageInfoBackward;
        };
      };
    };
  };

  const paginatedOctokit = paginateGraphQL(octokit);
  return (_currentDeployment = paginatedOctokit.graphql
    .paginate<DeploymentQueryResponseType>(
      `
    query paginate($owner: String!, $repo: String!, $commit: GitObjectID!, $cursor: String) {
      repository(owner:$owner, name:$repo) {
        object(oid:$commit) {
          ... on Commit {
            deployments(last: 100, after: $cursor, orderBy: {field: CREATED_AT, direction: DESC}) {
              nodes {
                environment
                state
                statuses(last: 1) {
                  nodes {
                    environment
                    logUrl
                    state
                  }
                }
              }
              pageInfo {
                startCursor
                hasPreviousPage
              }
            }
          }
        }
      }
    }`,
      {
        ...context.repo, // `owner`, `repo`
        commit: context.sha,
      }
    )
    .then(
      ({
        repository: {
          object: {
            deployments: { nodes: deployments },
          },
        },
      }) => {
        const inProgressDeployments = deployments.filter(
          (deployment) => deployment.state === "IN_PROGRESS"
        );

        if (inProgressDeployments.length == 0) {
          throw new Error(
            `Failed to find in-progress deployment (action bug?)`
          );
        }

        const deploymentsForThisJob = inProgressDeployments.filter(
          (deployment) => {
            // The 'last: 1' part of the query limits this to a single item.
            // There must always be at least one item or otherwise the
            // deployment would not exist.
            const logUrl = deployment.statuses.nodes[0].logUrl;
            // Parse the log URL to extract the job and run IDs
            const logUrlMatch = logUrl.match(
              /runs\/(?<run_id>[^/]+)\/jobs?\/(?<job_id>[^/]+)$/m
            );

            if (logUrlMatch === null) {
              throw new Error(`Log URL "${logUrl}" did not match regex`);
            }

            return (
              logUrlMatch.groups?.run_id === context.runId.toString() &&
              logUrlMatch.groups?.job_id ===
                currentWorkflowJob.databaseId.toString()
            );
          }
        );

        if (deploymentsForThisJob.length == 0) {
          throw new Error(
            `Failed to find in-progress deployment for this job (action bug?)`
          );
        }

        if (deploymentsForThisJob.length !== 1) {
          throw new Error(
            `Found more than one in-progress deployment for this job (action bug?)`
          );
        }

        return deploymentsForThisJob[0];
      }
    ));
}

// TODO rewrite

/**
 * Throw a permission error
 * @param permission - GitHub Job permission
 * @param options - error options
 * @returns void
 */
function throwPermissionError(
  permission: { scope: string; permission: string },
  options?: ErrorOptions
): never {
  throw new Error(
    `Ensure that GitHub job has permission: \`${permission.scope}: ${permission.permission}\`. ` +
      "https://docs.github.com/en/actions/security-guides/automatic-token-authentication#modifying-the-permissions-for-the-github_token",
    options
  );
}
