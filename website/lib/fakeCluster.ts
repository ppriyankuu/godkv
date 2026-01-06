import { Node } from "@/types";

export function createDemoCluster(): Node[] {
    return [
        { id: "node-a", angle: 20 },
        { id: "node-b", angle: 95 },
        { id: "node-c", angle: 165 },
        { id: "node-d", angle: 240 },
        { id: "node-e", angle: 310 },
    ];
}

export function demoKeyAngle(key: string): number {
    let sum = 0;
    for (let i = 0; i < key.length; i++) {
        sum += key.charCodeAt(i);
    }

    return (sum * 13) % 360;
}

export function demoReplicas(
    nodes: Node[],
    keyAngle: number,
    replicationFactor: number
): Node[] {
    const sorted = [...nodes].sort((a, b) => a.angle - b.angle);

    let start = sorted.findIndex(n => n.angle >= keyAngle);
    if (start === -1) start = 0;

    return Array.from({ length: replicationFactor }, (_, i) => {
        const node = sorted[(start + i) % sorted.length];
        return {
            ...node,
            role: i === 0 ? "primary" : "replica",
        };
    });
}

export function simulateKeyPlacement(
    key: string,
    rf: number
): {
    keyAngle: number;
    owners: Node[];
} {
    const nodes = createDemoCluster();
    const keyAngle = demoKeyAngle(key);
    const owners = demoReplicas(nodes, keyAngle, rf);

    return { keyAngle, owners };
}

