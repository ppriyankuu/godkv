import { Node } from "@/types";

const RING_RADIUS = 140;
const CENTER = 160;
const KEY_RADIUS = 6;
const NODE_RADIUS = 8;

interface HashRingProps {
    nodes: Node[];
    keyAngle: number;
    owners: Node[];
    keyName: string;
}

export function HashRing({
    nodes,
    keyAngle,
    owners,
    keyName,
}: HashRingProps) {
    const angleToCoord = (angle: number) => {
        const rad = ((angle - 90) * Math.PI) / 180;

        const x = CENTER + RING_RADIUS * Math.cos(rad);
        const y = CENTER + RING_RADIUS * Math.sin(rad);

        return {
            x: Number(x.toFixed(2)),
            y: Number(y.toFixed(2)),
        };
    };

    const keyPos = angleToCoord(keyAngle);

    const colorForNode = (node: Node) => {
        switch (node.role) {
            case "primary":
                return "#10b981"; // emerald-500
            case "replica":
                return "#34d399"; // emerald-300
            default:
                return "#6b7280"; // zinc-500
        }
    };

    return (
        <div className="bg-zinc-900/40 border border-zinc-800 rounded-xl p-4 sm:p-6">
            <div className="aspect-square max-w-md mx-auto">
                <svg viewBox="0 0 320 320" className="w-full h-full">
                    {/* Outer ring */}
                    <circle
                        cx={CENTER}
                        cy={CENTER}
                        r={RING_RADIUS}
                        fill="none"
                        stroke="#3f3f46"
                        strokeWidth="1.5"
                        className="opacity-60"
                    />

                    {/* Nodes */}
                    {nodes.map((node) => {
                        const { x, y } = angleToCoord(node.angle);

                        return (
                            <g key={node.id}>
                                <circle
                                    cx={x}
                                    cy={y}
                                    r={NODE_RADIUS}
                                    fill={colorForNode(node)}
                                    stroke="#000"
                                    strokeWidth="0.5"
                                    className="transition-all"
                                />

                                <text
                                    x={x}
                                    y={y - 14}
                                    textAnchor="middle"
                                    className="text-[10px] fill-zinc-300 font-mono"
                                >
                                    {node.id}
                                </text>

                                <text
                                    x={x}
                                    y={y + NODE_RADIUS + 12}
                                    textAnchor="middle"
                                    className="text-[9px] fill-zinc-500"
                                >
                                    {node.angle.toFixed(0)}°
                                </text>
                            </g>
                        );
                    })}

                    {/* Key marker */}
                    <circle
                        cx={keyPos.x}
                        cy={keyPos.y}
                        r={KEY_RADIUS}
                        fill="#f87171"
                        stroke="#000"
                        strokeWidth="0.5"
                    />
                    <text
                        x={keyPos.x}
                        y={keyPos.y - KEY_RADIUS - 8}
                        textAnchor="middle"
                        className="text-[11px] fill-red-300 font-mono"
                    >
                        {keyName}
                    </text>

                    {/* Directional arc: key → last replica */}
                    {owners.length > 0 && (() => {
                        const start = keyAngle;
                        let end = owners[owners.length - 1].angle;

                        if (end < start) end += 360;

                        const largeArcFlag = end - start > 180 ? 1 : 0;
                        const startCoord = angleToCoord(start);
                        const endCoord = angleToCoord(end % 360);

                        return (
                            <path
                                d={`M ${startCoord.x} ${startCoord.y}
                                    A ${RING_RADIUS} ${RING_RADIUS}
                                    0 ${largeArcFlag} 1
                                    ${endCoord.x} ${endCoord.y}`}
                                fill="none"
                                stroke="#10b981"
                                strokeWidth="2"
                                strokeDasharray="5,4"
                                className="opacity-70"
                            />
                        );
                    })()}
                </svg>
            </div>

            <div className="mt-3 text-center text-xs text-zinc-500">
                {'->'} Key is written to the primary node and replicated clockwise
            </div>
        </div>
    );
}
