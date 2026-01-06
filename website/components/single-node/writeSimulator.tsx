import { useState } from "react";

interface WriteSimulatorProps {
    onPut: (key: string, value: string) => void;
    onDelete: (key: string) => void;
}

export function WriteSimulator({ onPut, onDelete }: WriteSimulatorProps) {
    const [key, setKey] = useState("");
    const [value, setValue] = useState("");
    const [op, setOp] = useState<"PUT" | "DELETE">("PUT");

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        if (!key.trim()) return;

        if (op === "PUT" && value.trim()) {
            onPut(key.trim(), value.trim());
        } else if (op === "DELETE") {
            onDelete(key.trim());
        }

        setKey("");
        setValue("");
    };

    return (
        <form onSubmit={handleSubmit} className="flex flex-col text-zinc-400 sm:flex-row gap-2 w-full">
            <div className="flex gap-1">
                <button
                    type="button"
                    className={`px-2 py-1 text-xs rounded ${op === "PUT"
                        ? "bg-blue-600 text-white"
                        : "bg-zinc-800 text-zinc-400 hover:bg-zinc-700"
                        }`}
                    onClick={() => setOp("PUT")}
                >
                    PUT
                </button>
                <button
                    type="button"
                    className={`px-2 py-1 text-xs rounded ${op === "DELETE"
                        ? "bg-rose-600 text-white"
                        : "bg-zinc-800 text-zinc-400 hover:bg-zinc-700"
                        }`}
                    onClick={() => setOp("DELETE")}
                >
                    DELETE
                </button>
            </div>

            <input
                type="text"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder="Key (e.g. user:123)"
                className="flex-1 text-sm px-2 py-1 bg-zinc-800 border border-zinc-700 rounded focus:outline-none focus:ring-1 focus:ring-blue-500"
                required
            />

            {op === "PUT" && (
                <input
                    type="text"
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                    placeholder="Value"
                    className="flex-1 text-sm px-2 py-1 bg-zinc-800 border border-zinc-700 rounded focus:outline-none focus:ring-1 focus:ring-blue-500"
                    required
                />
            )}

            <button
                type="submit"
                className="px-3 py-1 bg-zinc-700 hover:bg-zinc-600 text-white text-sm rounded whitespace-nowrap"
            >
                Execute
            </button>
        </form>
    );
}