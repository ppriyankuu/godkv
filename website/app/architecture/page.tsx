
export default function ArchitecturePage() {
    return (
        <div className="space-y-8 py-8">
            <div>
                <h1 className="text-2xl font-bold text-zinc-100">System Architecture</h1>
                <p className="text-zinc-400 mt-2">
                    See how all components — WAL, hash ring, quorum — fit together in a distributed key-value store.
                </p>
            </div>

            {/* Overview */}
            <div className="bg-zinc-900/60 backdrop-blur-sm border border-zinc-800 rounded-xl p-6">
                <h2 className="text-lg font-semibold text-zinc-200 mb-3">How It Works</h2>
                <p className="text-sm text-zinc-400 leading-relaxed">
                    Each node runs an independent key-value engine using an in-memory table for fast reads, a Write-Ahead Log (WAL) for durability,
                    and periodic snapshots to compact state. When you scale to multiple nodes, a <strong>hash ring</strong> maps keys to nodes,
                    and writes require acknowledgment from a <strong>quorum</strong> (majority) of replicas to stay consistent even if some nodes fail.
                </p>
            </div>

            {/* Core Components per Node */}
            <div>
                <h2 className="text-lg font-semibold text-zinc-200 mb-4">Inside One Node</h2>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                    <ComponentCard
                        title="Memory Table"
                        color="blue"
                        desc="Fast in-memory storage for recent writes. Serves reads instantly."
                    />
                    <ComponentCard
                        title="WAL (Write-Ahead Log)"
                        color="amber"
                        desc="Append-only log on disk. Ensures durability: no data loss on crash."
                    />
                    <ComponentCard
                        title="Snapshot"
                        color="emerald"
                        desc="Compact, point-in-time backup of the state. Used to truncate old WAL entries."
                    />
                </div>
            </div>

            {/* Distributed Layer */}
            <div>
                <h2 className="text-lg font-semibold text-zinc-200 mb-4">Across Multiple Nodes</h2>
                <div className="space-y-4">
                    <ComponentCard
                        title="Consistent Hash Ring"
                        color="violet"
                        desc="Maps every key to 2+ physical nodes using consistent hashing. Adding/removing nodes minimally redistributes keys."
                    />
                    <ComponentCard
                        title="Replication & Quorum"
                        color="rose"
                        desc="Each write goes to the primary + replicas. A quorum (e.g., 2 of 3) must acknowledge before success. Prevents split-brain."
                    />
                    <ComponentCard
                        title="Failure Handling"
                        color="orange"
                        desc="If a node dies, traffic reroutes via the hash ring. Snapshots + WAL let new nodes catch up quickly."
                    />
                </div>
            </div>

            {/* Data Flow Example */}
            <div className="bg-zinc-900/60 backdrop-blur-sm border border-zinc-800 rounded-xl p-6">
                <h2 className="text-lg font-semibold text-zinc-200 mb-3">Example: Writing 'user:123'</h2>
                <ol className="list-decimal list-inside space-y-2 text-sm text-zinc-400">
                    <li>Key <code className="bg-zinc-800 px-1 rounded">user:123</code> is hashed to angle 142.7° on the ring.</li>
                    <li>Hash ring identifies Node B (145°) as primary and Node C (210°) as replica.</li>
                    <li>Write request sent to both. Each appends to WAL and updates memory table.</li>
                    <li>Once 2/2 nodes confirm, client gets success.</li>
                    <li>Periodically, each node snapshots its state and truncates old WAL.</li>
                </ol>
            </div>
        </div>
    );
}

function ComponentCard({
    title,
    desc,
    color,
}: {
    title: string;
    desc: string;
    color: "blue" | "emerald" | "amber" | "violet" | "rose" | "orange";
}) {
    const colorClasses = {
        blue: "text-blue-400 border-blue-900/50 bg-blue-900/20",
        emerald: "text-emerald-400 border-emerald-900/50 bg-emerald-900/20",
        amber: "text-amber-400 border-amber-900/50 bg-amber-900/20",
        violet: "text-violet-400 border-violet-900/50 bg-violet-900/20",
        rose: "text-rose-400 border-rose-900/50 bg-rose-900/20",
        orange: "text-orange-400 border-orange-900/50 bg-orange-900/20",
    };

    return (
        <div
            className={`rounded-lg border p-4 ${colorClasses[color]} backdrop-blur-sm`}
        >
            <div className="font-semibold text-sm">{title}</div>
            <div className="mt-1 text-xs text-zinc-400">{desc}</div>
        </div>
    );
}