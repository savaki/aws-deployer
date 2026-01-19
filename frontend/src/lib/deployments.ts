export interface DeploymentForSorting {
    name: string
    deployedAt: Date
}

/**
 * Sorts deployment names by their most recent deployment time.
 * Groups deployments by name, finds the most recent deployedAt for each,
 * and returns names sorted in descending order (most recent first).
 */
export function sortReposByRecentBuild<T extends DeploymentForSorting>(deployments: T[]): string[] {
    // Find the most recent deployedAt for each repo
    const repoLatestTime = new Map<string, Date>()
    deployments.forEach((d) => {
        const current = repoLatestTime.get(d.name)
        if (!current || d.deployedAt > current) {
            repoLatestTime.set(d.name, d.deployedAt)
        }
    })

    // Sort by most recent build time (descending)
    return Array.from(repoLatestTime.keys()).sort((a, b) => {
        const timeA = repoLatestTime.get(a)!.getTime()
        const timeB = repoLatestTime.get(b)!.getTime()
        return timeB - timeA
    })
}
