export type Maybe<T> = T | null;
export type InputMaybe<T> = Maybe<T>;
export type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
export type MakeOptional<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]?: Maybe<T[SubKey]> };
export type MakeMaybe<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]: Maybe<T[SubKey]> };
export type MakeEmpty<T extends { [key: string]: unknown }, K extends keyof T> = { [_ in K]?: never };
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
  ID: { input: string; output: string; }
  String: { input: string; output: string; }
  Boolean: { input: boolean; output: boolean; }
  Int: { input: number; output: number; }
  Float: { input: number; output: number; }
  /** Custom scalar for date-time values in ISO 8601 format */
  DateTime: { input: string; output: string; }
};

/** Build represents a deployment build */
export type Build = {
  __typename?: 'Build';
  /** Git branch */
  branch: Scalars['String']['output'];
  /** Build number from version */
  buildNumber: Scalars['String']['output'];
  /** Git commit hash */
  commitHash: Scalars['String']['output'];
  /** Deployment errors from multi-account deployments */
  deploymentErrors: Array<DeploymentError>;
  /** Downstream environments configured for promotion */
  downstreamEnvs: Array<Scalars['String']['output']>;
  /** Timestamp when build ended (if completed) */
  endTime?: Maybe<Scalars['DateTime']['output']>;
  /** Environment name (dev, staging, prod) */
  env: Scalars['String']['output'];
  /** Error message (if failed) */
  errorMsg?: Maybe<Scalars['String']['output']>;
  /** Step Functions execution ARN */
  executionArn?: Maybe<Scalars['String']['output']>;
  /** Unique build ID in format: {repo}/{env}:{ksuid} */
  id: Scalars['ID']['output'];
  /** Repository name */
  repo: Scalars['String']['output'];
  /** CloudFormation stack name */
  stackName: Scalars['String']['output'];
  /** Timestamp when build started */
  startTime: Scalars['DateTime']['output'];
  /** Current build status */
  status: BuildStatus;
  /** Version string */
  version: Scalars['String']['output'];
};

/** Build status enum representing the current state of a build */
export type BuildStatus =
  | 'FAILED'
  | 'IN_PROGRESS'
  | 'PENDING'
  | 'SUCCESS';

/** Deployment error from a multi-account deployment */
export type DeploymentError = {
  __typename?: 'DeploymentError';
  /** AWS Account ID */
  accountId: Scalars['String']['output'];
  /** AWS Region */
  region: Scalars['String']['output'];
  /** Recent failed stack events */
  stackEvents: Array<Scalars['String']['output']>;
  /** CloudFormation status reason */
  statusReason?: Maybe<Scalars['String']['output']>;
};

/** DeploymentTargets represents deployment configuration for a specific environment */
export type DeploymentTargets = {
  __typename?: 'DeploymentTargets';
  /** Downstream environments for promotion */
  downstreamEnvs: Array<Scalars['String']['output']>;
  /** Environment name */
  env: Scalars['String']['output'];
  /** Repository name (or '$' for default) */
  repo: Scalars['String']['output'];
  /** List of deployment targets */
  targets: Array<Target>;
};

export type Mutation = {
  __typename?: 'Mutation';
  /** Promote a build to downstream environments */
  promote: Query;
  /** Redeploy a specific version */
  redeploy: Query;
};


export type MutationPromoteArgs = {
  buildId: Scalars['ID']['input'];
};


export type MutationRedeployArgs = {
  buildId: Scalars['ID']['input'];
};

/** PipelineConfig represents the promotion structure for a repository */
export type PipelineConfig = {
  __typename?: 'PipelineConfig';
  /** All environment configurations in this pipeline */
  environments: Array<DeploymentTargets>;
  /** Initial environment */
  initialEnv: Scalars['String']['output'];
  /** Repository name (or '$' for default) */
  repo: Scalars['String']['output'];
};

export type Query = {
  __typename?: 'Query';
  /** List recent builds for a given environment */
  builds: Array<Build>;
  /** List all builds for a specific repository and environment */
  buildsByRepo: Array<Build>;
  /** Simple health check that returns "ok" */
  ok: Scalars['String']['output'];
  /** Get all pipeline configurations (default and per-repo) */
  pipelines: Array<PipelineConfig>;
};


export type QueryBuildsArgs = {
  env: Scalars['String']['input'];
};


export type QueryBuildsByRepoArgs = {
  env: Scalars['String']['input'];
  repo: Scalars['String']['input'];
};

/** Target represents account IDs and regions for deployment */
export type Target = {
  __typename?: 'Target';
  /** List of AWS Account IDs */
  accountIds: Array<Scalars['String']['output']>;
  /** List of AWS Regions */
  regions: Array<Scalars['String']['output']>;
};

export type BuildsQueryVariables = Exact<{
  env: Scalars['String']['input'];
}>;


export type BuildsQuery = { __typename?: 'Query', builds: Array<{ __typename?: 'Build', id: string, repo: string, env: string, buildNumber: string, branch: string, version: string, commitHash: string, status: BuildStatus, stackName: string, executionArn?: string | null, downstreamEnvs: Array<string>, startTime: string, endTime?: string | null, errorMsg?: string | null, deploymentErrors: Array<{ __typename?: 'DeploymentError', accountId: string, region: string, statusReason?: string | null, stackEvents: Array<string> }> }> };

export type BuildsByRepoQueryVariables = Exact<{
  repo: Scalars['String']['input'];
  env: Scalars['String']['input'];
}>;


export type BuildsByRepoQuery = { __typename?: 'Query', buildsByRepo: Array<{ __typename?: 'Build', id: string, repo: string, env: string, buildNumber: string, branch: string, version: string, commitHash: string, status: BuildStatus, stackName: string, executionArn?: string | null, downstreamEnvs: Array<string>, startTime: string, endTime?: string | null, errorMsg?: string | null, deploymentErrors: Array<{ __typename?: 'DeploymentError', accountId: string, region: string, statusReason?: string | null, stackEvents: Array<string> }> }> };

export type PromoteMutationVariables = Exact<{
  buildId: Scalars['ID']['input'];
}>;


export type PromoteMutation = { __typename?: 'Mutation', promote: { __typename?: 'Query', ok: string } };

export type PipelinesQueryVariables = Exact<{ [key: string]: never; }>;


export type PipelinesQuery = { __typename?: 'Query', pipelines: Array<{ __typename?: 'PipelineConfig', repo: string, initialEnv: string, environments: Array<{ __typename?: 'DeploymentTargets', repo: string, env: string, downstreamEnvs: Array<string>, targets: Array<{ __typename?: 'Target', accountIds: Array<string>, regions: Array<string> }> }> }> };
