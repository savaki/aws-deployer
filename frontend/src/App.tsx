import {Router, Route, A, useLocation} from '@solidjs/router'
import {createSignal, Show} from 'solid-js'
import {TbActivity, TbGitBranch} from 'solid-icons/tb'
import {IoClose, IoSearch} from 'solid-icons/io'
import {DeploymentsPage} from './pages/DeploymentsPage'
import {EnvironmentsPage} from './pages/EnvironmentsPage'
import {ToastRegion, ToastList} from './components/ui/toast'

// Global filter state
const [filterText, setFilterText] = createSignal('')

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
            <Route path="/" component={() => <DeploymentsPage filterText={filterText()} />}/>
            <Route path="/envs" component={EnvironmentsPage}/>
        </Router>
    )
}

function Header() {
    const location = useLocation()
    const isActive = (path: string) => location.pathname === path

    return (
        <div class="mb-4">
            <div class="flex items-center gap-2 mb-1.5 flex-wrap">
                {/* Left: Logo + Title */}
                <div class="flex items-center gap-2">
                    <TbActivity class="h-6 w-6 text-primary"/>
                    <h1 class="text-2xl font-bold tracking-tight">AWS Deployer</h1>
                </div>

                {/* Center: Filter input - grows to fill available space */}
                <div class="flex-1 flex justify-center">
                    <div class="relative group">
                        <IoSearch class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground transition-colors group-focus-within:text-primary" />
                        <input
                            type="text"
                            placeholder="Filter repos..."
                            value={filterText()}
                            onInput={(e) => setFilterText(e.currentTarget.value)}
                            class="pl-9 pr-8 py-2 text-sm bg-card border border-border rounded-lg shadow-sm transition-all duration-200 ease-out w-56 placeholder:text-muted-foreground/60 focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary hover:border-muted-foreground/30"
                        />
                        <Show when={filterText()}>
                            <button
                                onClick={() => setFilterText('')}
                                class="absolute right-2 top-1/2 -translate-y-1/2 p-0.5 rounded text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                                aria-label="Clear filter"
                            >
                                <IoClose class="w-4 h-4" />
                            </button>
                        </Show>
                    </div>
                </div>

                {/* Right: Desktop navigation - hidden on mobile */}
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
