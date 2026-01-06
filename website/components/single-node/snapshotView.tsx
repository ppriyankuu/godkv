import { Snapshot } from "@/types";
import { ClientTime } from "../layout/time";

interface SnapshotViewProps {
    snapshot: Snapshot | null;
}

export function SnapshotView({ snapshot }: SnapshotViewProps) {
    if (!snapshot || Object.keys(snapshot.entries).length === 0) {
        return (
            <p className="text-zinc-500 text-sm italic">
                No snapshot yet. Snapshots are taken periodically or after many writes.
            </p>
        );
    }

    const entries = Object.values(snapshot.entries);

    return (
        <div className="space-y-2 max-h-40 overflow-y-auto">
            <p className="text-xs text-zinc-500 mb-1">
                Taken at: <ClientTime timestamp={snapshot.takenAt} />
            </p>
            {entries.map((entry) => (
                <div
                    key={entry.key}
                    className="text-xs bg-zinc-800/50 px-3 py-1.5 rounded border border-zinc-800 flex justify-between font-mono"
                >
                    <span className="text-blue-300">{entry.key}</span>
                    <span className="text-green-300">"{entry.value}"</span>
                    <span className="text-zinc-500">v{entry.version}</span>
                </div>
            ))}
        </div>
    );
}