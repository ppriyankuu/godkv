import { KVEntry } from "@/types";

interface MemoryViewProps {
    entries: KVEntry[];
}

export function MemoryView({ entries }: MemoryViewProps) {
    if (entries.length === 0) {
        return (
            <p className="text-zinc-500 text-sm italic">
                Memory is empty. Writes appear here after being logged in WAL.
            </p>
        );
    }

    return (
        <div className="space-y-2 max-h-60 overflow-y-auto">
            {entries.map((entry) => (
                <div
                    key={entry.key}
                    className="text-sm bg-zinc-800/50 px-3 py-2 rounded border border-zinc-800 flex justify-between font-mono"
                >
                    <span className="text-blue-300">{entry.key}</span>
                    <span className="text-green-300">"{entry.value}"</span>
                    <span className="text-zinc-500 text-xs">v{entry.version}</span>
                </div>
            ))}
        </div>
    );
}