import { MemtableInfo } from "../memoryTableInfo";
import { SnapshotInfo } from "../snapshotInfo";
import { WALInfo } from "../WALinfo";

export function SingleNodeInfo() {
    return (
        <header className="space-y-4 mt-14 lg:mt-12">
            <h1 className="text-2xl text-zinc-300 font-bold">Single-Node Key-Value Store</h1>

            <div className="bg-zinc-900/50 border border-zinc-800 rounded-lg p-4 max-w-3xl">
                <p className="text-zinc-300 mb-3">
                    This is a simple but reliable key-value store running on a single machine.
                    It is designed to be fast, and to never lose data even if the program crashes.
                </p>

                <p className="text-zinc-300 mb-4">
                    To do this, it uses three main parts:
                    a <WALInfo /> to safely record every change,
                    a <MemtableInfo /> to quickly store data in memory,
                    and <SnapshotInfo /> to save the current state to disk from time to time.
                </p>

                <ul className="text-sm text-zinc-400 space-y-2 list-disc pl-5">
                    <li>
                        <strong>When writing data: </strong>
                        The change is first written to disk (WAL) so it is never lost,
                        and then stored in memory for fast access.
                    </li>
                    <li>
                        <strong>When reading data: </strong>
                        The store looks in memory first because it is the fastest place to check.
                    </li>
                    <li>
                        <strong>If the server crashes: </strong>
                        It restores the last saved snapshot and replays recent changes from the WAL.
                    </li>
                    <li>
                        <strong>Cleanup over time: </strong>
                        Old data and deleted keys are cleaned up to keep storage efficient.
                    </li>
                </ul>
            </div>
        </header>
    );
}
