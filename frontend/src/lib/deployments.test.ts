import {describe, it, expect} from 'vitest'
import {sortReposByRecentBuild} from './deployments'

describe('sortReposByRecentBuild', () => {
    it('sorts repos by most recent deployment time (descending)', () => {
        const deployments = [
            {name: 'repo-a', deployedAt: new Date('2024-01-01T10:00:00Z')},
            {name: 'repo-b', deployedAt: new Date('2024-01-03T10:00:00Z')},
            {name: 'repo-c', deployedAt: new Date('2024-01-02T10:00:00Z')},
        ]

        const result = sortReposByRecentBuild(deployments)

        expect(result).toEqual(['repo-b', 'repo-c', 'repo-a'])
    })

    it('uses most recent deployment when repo has multiple deployments', () => {
        const deployments = [
            {name: 'repo-a', deployedAt: new Date('2024-01-01T10:00:00Z')}, // dev
            {name: 'repo-a', deployedAt: new Date('2024-01-05T10:00:00Z')}, // stg - most recent for repo-a
            {name: 'repo-a', deployedAt: new Date('2024-01-02T10:00:00Z')}, // prd
            {name: 'repo-b', deployedAt: new Date('2024-01-03T10:00:00Z')}, // only one env
        ]

        const result = sortReposByRecentBuild(deployments)

        // repo-a's most recent is Jan 5, repo-b is Jan 3
        expect(result).toEqual(['repo-a', 'repo-b'])
    })

    it('handles empty deployments array', () => {
        const result = sortReposByRecentBuild([])
        expect(result).toEqual([])
    })

    it('handles single deployment', () => {
        const deployments = [{name: 'only-repo', deployedAt: new Date('2024-01-01T10:00:00Z')}]

        const result = sortReposByRecentBuild(deployments)

        expect(result).toEqual(['only-repo'])
    })

    it('handles deployments with same timestamp', () => {
        const sameTime = new Date('2024-01-01T10:00:00Z')
        const deployments = [
            {name: 'repo-a', deployedAt: sameTime},
            {name: 'repo-b', deployedAt: sameTime},
        ]

        const result = sortReposByRecentBuild(deployments)

        // Both have same time, order is stable from Map iteration
        expect(result).toHaveLength(2)
        expect(result).toContain('repo-a')
        expect(result).toContain('repo-b')
    })

    it('correctly compares across multiple environments per repo', () => {
        const deployments = [
            // repo-old: was deployed to all envs but long ago
            {name: 'repo-old', deployedAt: new Date('2023-01-01T10:00:00Z')},
            {name: 'repo-old', deployedAt: new Date('2023-02-01T10:00:00Z')},
            {name: 'repo-old', deployedAt: new Date('2023-03-01T10:00:00Z')}, // most recent for repo-old

            // repo-new: only deployed to dev but very recently
            {name: 'repo-new', deployedAt: new Date('2024-06-01T10:00:00Z')},

            // repo-mid: deployed a month ago
            {name: 'repo-mid', deployedAt: new Date('2024-05-01T10:00:00Z')},
        ]

        const result = sortReposByRecentBuild(deployments)

        expect(result).toEqual(['repo-new', 'repo-mid', 'repo-old'])
    })
})
