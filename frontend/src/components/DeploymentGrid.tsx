import {For} from 'solid-js'
import type {DeploymentStatus} from './DeploymentCard'
import {DeploymentCard} from './DeploymentCard'

export interface DeploymentError {
    accountId: string
    region: string
    statusReason?: string | null
    stackEvents: string[]
}

export interface Deployment {
    id: string
    name: string
    version: string
    environment: string
    status: DeploymentStatus
    deployedAt: Date
    failureReason?: string
    stackName?: string
    executionArn?: string
    downstreamEnvs: string[]
    deploymentErrors: DeploymentError[]
}

export interface RedeployInput {
    buildId: string
    version: string
    name: string
    environment: string
}

interface DeploymentGridProps {
    deployments: Deployment[]
    versionHistory: Record<string, string[]>
    onRedeploy: (input: RedeployInput) => void
    onPromote: (deployment: Deployment) => void
    selectedEnv?: string // For mobile single-env view
}

const ENVIRONMENTS = ['dev', 'stg', 'prd']

const ENV_LABELS: Record<string, string> = {
    dev: 'Development',
    stg: 'Staging',
    prd: 'Production',
}

export function DeploymentGrid(props: DeploymentGridProps) {
    // Group deployments by build name
    const buildNames = () => {
        const names = new Set(props.deployments.map((d) => d.name))
        return Array.from(names).sort()
    }

    // Create a lookup map for deployments
    const deploymentMap = () => {
        const map = new Map<string, Deployment>()
        props.deployments.forEach((d) => {
            map.set(`${d.name}-${d.environment}`, d)
        })
        return map
    }

    // Create a simplified map for version comparison (for canPromote logic)
    const versionMap = () => {
        const map = new Map<string, {version: string; env: string; deployedAt: Date}>()
        props.deployments.forEach((d) => {
            map.set(`${d.name}-${d.environment}`, {version: d.version, env: d.environment, deployedAt: d.deployedAt})
        })
        return map
    }

    return (
        <div class="space-y-2">
            {/* Header Row - hidden on mobile */}
            <div class="desktop-grid-header grid-cols-[200px_1fr] gap-2">
                <div class="font-semibold text-sm text-muted-foreground uppercase tracking-wide">
                    Build Name
                </div>
                <div class="desktop-grid-header grid-cols-3 gap-2">
                    <For each={ENVIRONMENTS}>
                        {(env) => (
                            <div
                                class="font-semibold text-sm text-muted-foreground uppercase tracking-wide text-center">
                                {ENV_LABELS[env]}
                            </div>
                        )}
                    </For>
                </div>
            </div>

            {/* Build Rows - Desktop: 3-column grid, Mobile: single column */}
            <For each={buildNames()}>
                {(buildName) => (
                    <div class="deployment-row">
                        {/* Desktop layout */}
                        <div class="desktop-layout grid-cols-[200px_1fr] gap-2 items-start">
                            <div class="pt-2">
                                <h3 class="font-semibold text-lg">{buildName}</h3>
                            </div>
                            <div class="desktop-layout grid-cols-3 gap-2">
                                <For each={ENVIRONMENTS}>
                                    {(env) => {
                                        const deployment = deploymentMap().get(`${buildName}-${env}`)
                                        return (
                                            <DeploymentCard
                                                buildId={deployment?.id}
                                                version={deployment?.version}
                                                status={deployment?.status}
                                                deployedAt={deployment?.deployedAt}
                                                failureReason={deployment?.failureReason}
                                                environment={env}
                                                buildName={buildName}
                                                stackName={deployment?.stackName}
                                                executionArn={deployment?.executionArn}
                                                downstreamEnvs={deployment?.downstreamEnvs}
                                                deploymentErrors={deployment?.deploymentErrors}
                                                allDeployments={versionMap()}
                                                onRedeploy={(buildId, version) => props.onRedeploy({buildId, version, name: buildName, environment: env})}
                                                onPromote={() => deployment && props.onPromote(deployment)}
                                            />
                                        )
                                    }}
                                </For>
                            </div>
                        </div>

                        {/* Mobile layout - single column, selected env only */}
                        <div class="mobile-layout">
                            <div class="mb-2">
                                <h3 class="font-semibold text-lg">{buildName}</h3>
                            </div>
                            <div class="space-y-2">
                                {(() => {
                                    const env = props.selectedEnv || 'dev'
                                    const deployment = deploymentMap().get(`${buildName}-${env}`)
                                    return (
                                        <DeploymentCard
                                            buildId={deployment?.id}
                                            version={deployment?.version}
                                            status={deployment?.status}
                                            deployedAt={deployment?.deployedAt}
                                            failureReason={deployment?.failureReason}
                                            environment={env}
                                            buildName={buildName}
                                            stackName={deployment?.stackName}
                                            executionArn={deployment?.executionArn}
                                            downstreamEnvs={deployment?.downstreamEnvs}
                                            deploymentErrors={deployment?.deploymentErrors}
                                            allDeployments={versionMap()}
                                            onRedeploy={(buildId, version) => props.onRedeploy({buildId, version, name: buildName, environment: env})}
                                            onPromote={() => deployment && props.onPromote(deployment)}
                                        />
                                    )
                                })()}
                            </div>
                        </div>
                    </div>
                )}
            </For>
        </div>
    )
}
