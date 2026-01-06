import { WALRecord } from "@/types";
import { ClientTime } from "../layout/time";

interface WALViewProps {
    records: WALRecord[];
}

export function WALView({ records }: WALViewProps) {
    if (records.length === 0) {
        return (
            <p className="text-zinc-500 text-sm italic">
                No writes yet. Perform a PUT or DELETE to see entries here.
            </p>
        );
    }

    return (
        <div className="space-y-2 max-h-60 overflow-y-auto">
            {[...records].reverse().map((record, i) => (
                <div
                    key={i}
                    className="text-xs bg-zinc-800/50 px-3 py-2 rounded border border-zinc-800 flex justify-between"
                >
                    <span>
                        <span className="font-mono text-amber-400">{record.op}</span>{" "}
                        <span className="font-mono text-blue-300">{record.key}</span>
                        {record.value !== undefined && (
                            <span className="ml-1 text-green-300">= "{record.value}"</span>
                        )}
                    </span>
                    <span className="text-zinc-500">
                        <ClientTime timestamp={record.timestamp} />
                    </span>
                </div>
            ))}
        </div>
    );
}