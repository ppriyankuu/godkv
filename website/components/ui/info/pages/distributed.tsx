import { ConsistentHashingInfo } from "../consistentHashingInfo";
import { HashRingInfo } from "../hashRingInfo";
import { KeyInfo } from "../keyInfo";
import { ReplicationInfo } from "../replicationInfo";

export function DistributedInfo() {
    return (
        <header className="text-zinc-300 flex justify-center">

            <div className="space-y-4">
                <h1 className="text-2xl font-bold">Distributed Key-Value Store</h1>

                <div className="bg-zinc-900/50 border border-zinc-800 rounded-lg p-4 max-w-3xl">
                    <p className="text-zinc-300 mb-3">
                        When one machine isn&apos;t enough, we spread data across many nodes.
                        But adding or removing a node shouldn&apos;t require moving all data.
                        This system uses <ConsistentHashingInfo /> to minimize disruption,
                        and <ReplicationInfo /> to keep data safe if a node fails.
                    </p>

                    <p className="text-zinc-300 mb-4">
                        All keys and nodes live on a circular <HashRingInfo /> (0–360°).
                        A <KeyInfo /> is assigned to the first N nodes clockwise—those nodes store its data.
                        This demo uses a replication factor of 3.
                    </p>

                    <ul className="text-sm text-zinc-400 space-y-2 list-disc pl-5">
                        <li>
                            <strong>Adding a node: </strong>
                            Only a small fraction of keys move—just those now closer to the new node.
                        </li>
                        <li>
                            <strong>Removing a node: </strong>
                            Its keys are reassigned to the next nodes clockwise; other data stays put.
                        </li>
                        <li>
                            <strong>Reading or writing a key: </strong>
                            The client computes the key’s position and contacts its replica nodes.
                        </li>
                        <li>
                            <strong>Failure tolerance: </strong>
                            As long as one replica is alive, the data remains available.
                        </li>
                    </ul>
                </div>

            </div>
        </header>
    );
}