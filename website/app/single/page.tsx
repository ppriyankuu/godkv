'use client'

import { useEffect, useState } from "react";
import { MemoryView } from "@/components/single-node/memoryView";
import { SnapshotView } from "@/components/single-node/snapshotView";
import { WALView } from "@/components/single-node/WALView";
import { MemtableInfo } from "@/components/ui/info/memoryTableInfo";
import { SingleNodeInfo } from "@/components/ui/info/pages/single";
import { SnapshotInfo } from "@/components/ui/info/snapshotInfo";
import { WALInfo } from "@/components/ui/info/WALinfo";
import { KVEntry, Snapshot, WALRecord } from "@/types";
import { WriteSimulator } from "@/components/single-node/writeSimulator";

// Initial mocks
const initialWAL: WALRecord[] = [
    { op: "PUT", key: "user:100", value: "Alice", timestamp: Date.now() - 5000 },
    { op: "PUT", key: "user:101", value: "Bob", timestamp: Date.now() - 3000 },
    { op: "DELETE", key: "temp:flag", timestamp: Date.now() - 1000 },
];

const initialMemtable: KVEntry[] = [
    { key: "user:100", value: "Alice", version: 1 },
    { key: "user:101", value: "Bob", version: 1 },
];

const initialSnapshot: Snapshot = {
    takenAt: Date.now() - 10000,
    entries: {
        "user:100": { key: "user:100", value: "Alice", version: 1 },
    },
};

export default function SingleNodePage() {
    const [wal, setWAL] = useState<WALRecord[]>(initialWAL);
    const [memtable, setMemtable] = useState<KVEntry[]>(initialMemtable);
    const [snapshot, setSnapshot] = useState<Snapshot | null>(initialSnapshot);

    const handlePut = (key: string, value: string) => {
        const now = Date.now();
        const newRecord: WALRecord = { op: "PUT", key, value, timestamp: now };
        setWAL((prev) => [...prev, newRecord]);

        setMemtable((prev) => {
            const exists = prev.find((e) => e.key === key);
            if (exists) {
                return prev.map((e) =>
                    e.key === key ? { ...e, value, version: e.version + 1 } : e
                );
            }
            return [...prev, { key, value, version: 1 }];
        });
    };

    const handleDelete = (key: string) => {
        const now = Date.now();
        const newRecord: WALRecord = { op: "DELETE", key, timestamp: now };
        setWAL((prev) => [...prev, newRecord]);

        setMemtable((prev) => prev.filter((e) => e.key !== key));
    };

    useEffect(() => {
        if (wal.length >= 5) {
            const now = Date.now();

            const newSnapshot: Snapshot = {
                takenAt: now,
                entries: Object.fromEntries(
                    memtable.map((entry) => [entry.key, entry])
                ),
            };

            const truncatedWAL = wal.filter((record) => record.timestamp > now);

            setSnapshot(newSnapshot);
            setWAL(truncatedWAL);
        }
    }, [wal, memtable]);

    return (
        <div className="space-y-8 -mt-6 lg:-mt-4">
            <SingleNodeInfo />

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <div className="space-y-4">
                    <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
                        <h2 className="font-semibold text-zinc-200 mb-3 flex items-center gap-2">
                            <span className="h-2 w-2 rounded-full bg-blue-500 inline-block"></span>
                            <WALInfo />
                        </h2>
                        <p className="text-sm text-zinc-400 mb-4">
                            Every change is written here first so it is never lost.
                            If the server crashes, this log is used to rebuild what was last written.
                        </p>
                        <WALView records={wal} />
                    </div>

                    <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
                        <h2 className="font-semibold text-zinc-200 mb-3 flex items-center gap-2">
                            <span className="h-2 w-2 rounded-full bg-emerald-500 inline-block"></span>
                            <SnapshotInfo />
                        </h2>
                        <p className="text-sm text-zinc-400 mb-4">
                            A snapshot saves the full database state at a moment in time.
                            Older logs can be safely cleared once a snapshot is created.
                        </p>
                        <SnapshotView snapshot={snapshot} />
                    </div>
                </div>

                <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
                    <h2 className="font-semibold text-zinc-200 mb-3 flex items-center gap-2">
                        <span className="h-2 w-2 rounded-full bg-amber-500 inline-block"></span>
                        <MemtableInfo />
                    </h2>
                    <p className="text-sm text-zinc-400 mb-4">
                        This is where data lives while the server is running.
                        It allows very fast reads and writes, and when it gets too large,
                        the data is saved to disk and a fresh memory table is started.
                    </p>
                    <MemoryView entries={memtable} />
                </div>
            </div>

            <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
                <h3 className="font-medium text-zinc-200 mb-2">Simulate a Write</h3>
                <WriteSimulator onPut={handlePut} onDelete={handleDelete} />
            </div>

            <div className="bg-zinc-900/50 border border-zinc-800 rounded-lg p-4">
                <p className="pt-2 text-zinc-400">
                    Try performing a <code className="bg-zinc-800 px-1.5 py-0.5 rounded">PUT</code> or{" "}
                    <code className="bg-zinc-800 px-1.5 py-0.5 rounded">DELETE</code>—watch it appear in the WAL first,
                    then in memory. After 5 writes, a new snapshot is taken and the WAL is cleared.
                </p>
            </div>

            <div className="text-sm text-zinc-500 border-t border-zinc-800 pt-4 mb-10 text-center">
                <p>
                    <strong>Note:</strong> This visualization shows the core components. Real implementations add
                    features like compression, caching, and sophisticated compaction strategies.
                </p>
            </div>
        </div>
    );
}