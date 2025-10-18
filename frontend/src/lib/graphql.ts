import {GraphQLClient} from 'graphql-request'
import {createResource} from 'solid-js'
import type {BuildsQuery, BuildsQueryVariables, PipelinesQuery} from '../generated/graphql'

// Create GraphQL client pointing to the proxied endpoint
// Use window.location.origin to construct a full URL for graphql-request
const client = new GraphQLClient(`${window.location.origin}/graphql`)

// GraphQL query for fetching builds
export const BUILDS_QUERY = /* GraphQL */ `
  query Builds($env: String!) {
    builds(env: $env) {
      id
      repo
      env
      buildNumber
      branch
      version
      commitHash
      status
      stackName
      executionArn
      downstreamEnvs
      startTime
      endTime
      errorMsg
      deploymentErrors {
        accountId
        region
        statusReason
        stackEvents
      }
    }
  }
`

export type Build = BuildsQuery['builds'][0]

// Hook to fetch builds for a specific environment
export function createBuildsQuery(env: string) {
    const [data] = createResource<BuildsQuery>(async () => {
        return await client.request<BuildsQuery, BuildsQueryVariables>(
            BUILDS_QUERY,
            {env}
        )
    })

    return {
        builds: () => data()?.builds || [],
        loading: () => data.loading,
        error: () => data.error,
    }
}

// GraphQL query for fetching builds by repo and env
export const BUILDS_BY_REPO_QUERY = /* GraphQL */ `
  query BuildsByRepo($repo: String!, $env: String!) {
    buildsByRepo(repo: $repo, env: $env) {
      id
      repo
      env
      buildNumber
      branch
      version
      commitHash
      status
      stackName
      executionArn
      downstreamEnvs
      startTime
      endTime
      errorMsg
      deploymentErrors {
        accountId
        region
        statusReason
        stackEvents
      }
    }
  }
`

export interface BuildsByRepoVariables {
    repo: string
    env: string
}

export interface BuildsByRepoQuery {
    buildsByRepo: Build[]
}

// Function to fetch builds by repo and env
export async function fetchBuildsByRepo(repo: string, env: string): Promise<Build[]> {
    const result = await client.request<BuildsByRepoQuery, BuildsByRepoVariables>(
        BUILDS_BY_REPO_QUERY,
        { repo, env }
    )
    return result.buildsByRepo
}

// GraphQL mutation for promoting a build
export const PROMOTE_MUTATION = /* GraphQL */ `
  mutation Promote($buildId: ID!) {
    promote(buildId: $buildId) {
      ok
    }
  }
`

// Function to promote a build to downstream environments
export async function promoteDeployment(buildId: string): Promise<void> {
    await client.request(PROMOTE_MUTATION, { buildId })
}

// GraphQL mutation for redeploying a build
export const REDEPLOY_MUTATION = /* GraphQL */ `
  mutation Redeploy($buildId: ID!) {
    redeploy(buildId: $buildId) {
      ok
    }
  }
`

// Function to redeploy a specific build
export async function redeployBuild(buildId: string): Promise<void> {
    await client.request(REDEPLOY_MUTATION, { buildId })
}

// Utility to map BuildStatus enum to deployment status
export function mapBuildStatus(status: string): 'success' | 'failed' | 'in-progress' | 'pending' {
    switch (status) {
        case 'SUCCESS':
            return 'success'
        case 'FAILED':
            return 'failed'
        case 'IN_PROGRESS':
            return 'in-progress'
        case 'PENDING':
            return 'pending'
        default:
            return 'pending'
    }
}

// GraphQL query for fetching pipeline configurations
export const PIPELINES_QUERY = /* GraphQL */ `
  query Pipelines {
    pipelines {
      repo
      initialEnv
      environments {
        repo
        env
        targets {
          accountIds
          regions
        }
        downstreamEnvs
      }
    }
  }
`

export type PipelineConfig = PipelinesQuery['pipelines'][0]

// Hook to fetch all pipeline configurations
export function createPipelinesQuery() {
    const [data] = createResource<PipelinesQuery>(async () => {
        return await client.request<PipelinesQuery>(PIPELINES_QUERY)
    })

    return {
        pipelines: () => data()?.pipelines || [],
        loading: () => data.loading,
        error: () => data.error,
    }
}
