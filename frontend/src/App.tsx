import {Router, Route, A, useLocation} from '@solidjs/router'
import {TbActivity, TbGitBranch} from 'solid-icons/tb'
import {DeploymentsPage} from './pages/DeploymentsPage'
import {EnvironmentsPage} from './pages/EnvironmentsPage'
import {ToastRegion, ToastList} from './components/ui/toast'

function App() {
    return (
        <Router root={(props) => (
            <div class="min-h-screen bg-background">
                <div class="container mx-auto px-2 py-4 max-w-7xl">
                    <Header/>
                    {props.children}
                </div>
                <ToastRegion>
                    <ToastList />
                </ToastRegion>
            </div>
        )}>
            <Route path="/" component={DeploymentsPage}/>
            <Route path="/envs" component={EnvironmentsPage}/>
        </Router>
    )
}

function Header() {
    const location = useLocation()
    const isActive = (path: string) => location.pathname === path

    return (
        <div class="mb-4">
            <div class="flex items-center justify-between gap-2 mb-1.5 flex-wrap">
                <div class="flex items-center gap-2">
                    <TbActivity class="h-6 w-6 text-primary"/>
                    <h1 class="text-2xl font-bold tracking-tight">AWS Deployer</h1>
                </div>

                {/* Desktop navigation - hidden on mobile */}
                <nav class="desktop-nav flex items-center gap-4">
                    <A
                        href="/"
                        class={`flex items-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
                            isActive('/')
                                ? 'bg-primary/10 text-primary'
                                : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                        }`}
                    >
                        <TbActivity class="h-4 w-4"/>
                        Deployments
                    </A>
                    <A
                        href="/envs"
                        class={`flex items-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
                            isActive('/envs')
                                ? 'bg-primary/10 text-primary'
                                : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                        }`}
                    >
                        <TbGitBranch class="h-4 w-4"/>
                        Environments
                    </A>
                </nav>
            </div>
        </div>
    )
}

export default App
