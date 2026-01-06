"use client";

import { useState, useMemo } from "react";
import {
    createDemoCluster,
    simulateKeyPlacement,
} from "@/lib/fakeCluster";
import { HashRing } from "@/components/distributed/hashRing";
import { DistributedInfo } from "@/components/ui/info/pages/distributed";


export default function DistributedPage() {
    const nodes = useMemo(() => createDemoCluster(), []);
    const [key, setKey] = useState<string>("user:123");
    const [value, setValue] = useState<string>("hello");

    const { keyAngle, owners } = useMemo(() => {
        return simulateKeyPlacement(key, 2);
    }, [key]);

    return (
        <div className="space-y-8 py-8">
            <DistributedInfo />

            <div className="grid grid-cols-1 lg:grid-cols-[1fr_360px] gap-8 items-start">
                <div className="flex-1 min-w-0">
                    <HashRing
                        nodes={nodes}
                        keyAngle={keyAngle}
                        owners={owners}
                        keyName={key}
                    />
                </div>

                {/* Control Panel */}
                <div className="w-full lg:w-80 shrink-0 space-y-6">
                    <div className="bg-zinc-900/60 backdrop-blur-sm border border-zinc-800 rounded-xl p-5">
                        <h2 className="text-lg font-semibold text-zinc-200 mb-3">Simulate a Key</h2>
                        <div className="space-y-4">
                            <div>
                                <label className="text-xs text-zinc-400 uppercase tracking-wide">Key</label>
                                <input
                                    type="text"
                                    value={key}
                                    onChange={(e) => setKey(e.target.value)}
                                    placeholder="e.g. session:abc123"
                                    className="mt-1.5 w-full text-sm px-3 py-2.5 bg-zinc-800 border border-zinc-700 rounded-lg
                    focus:outline-none focus:ring-1 focus:ring-blue-500 text-zinc-100 placeholder-zinc-500"
                                />
                            </div>

                            <div>
                                <label className="text-xs text-zinc-400 uppercase tracking-wide">
                                    Value
                                </label>
                                <input
                                    type="text"
                                    value={value}
                                    onChange={(e) => setValue(e.target.value)}
                                    placeholder="e.g. hello world"
                                    className="mt-1.5 w-full text-sm px-3 py-2.5 bg-zinc-800 border border-zinc-700 rounded-lg
        focus:outline-none focus:ring-1 focus:ring-blue-500 text-zinc-100 placeholder-zinc-500"
                                />
                            </div>

                            <div>
                                <label className="text-xs text-zinc-400 uppercase tracking-wide">
                                    Hash Position
                                </label>
                                <div className="mt-1.5 text-sm text-zinc-300 font-mono bg-zinc-800/50 px-3 py-2 rounded">
                                    {keyAngle.toFixed(1)}°
                                </div>
                            </div>
                        </div>
                    </div>

                    <div className="bg-zinc-900/60 backdrop-blur-sm border border-zinc-800 rounded-xl p-5">
                        <div className="flex items-center gap-2 mb-3">
                            <span className="h-3 w-3 rounded-full bg-emerald-500"></span>
                            <h2 className="text-lg font-semibold text-zinc-200">Replica Nodes (RF = 2)</h2>
                        </div>
                        {owners.length === 0 ? (
                            <p className="text-zinc-500 italic text-sm">No nodes available</p>
                        ) : (
                            <ul className="space-y-2">
                                {owners.map((node, idx) => (
                                    <li
                                        key={node.id}
                                        className="p-2.5 bg-zinc-800/40 rounded-lg border border-zinc-700"
                                    >
                                        <div className="flex items-center gap-2">
                                            <span className="font-mono text-emerald-400 text-sm">
                                                {node.id}
                                            </span>
                                            <span className="text-[10px] uppercase text-zinc-500">
                                                {idx === 0 ? "PRIMARY" : "REPLICA"}
                                            </span>
                                            <span className="ml-auto text-xs text-zinc-500">
                                                {node.angle.toFixed(1)}°
                                            </span>
                                        </div>

                                        <div className="mt-1 text-xs text-zinc-400">
                                            stored value:{" "}
                                            <code className="bg-zinc-800 px-1 rounded text-zinc-200">
                                                {value}
                                            </code>
                                        </div>
                                    </li>
                                ))}
                            </ul>
                        )}
                        <p className="mt-4 text-xs text-zinc-500">
                            Data is replicated to the next 2 nodes clockwise from the key’s position.
                        </p>
                    </div>

                    <div className="text-xs text-zinc-500 border-t border-zinc-800 pt-4">
                        <p>
                            🔁 Try different keys like <code className="bg-zinc-800 px-1 rounded">user:456</code>,
                            <code className="bg-zinc-800 px-1 rounded">cache:xyz</code>, or random strings.
                        </p>
                    </div>
                </div>
            </div>
        </div>
    );
}