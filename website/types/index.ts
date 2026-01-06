export type KVOperation = 'PUT' | 'GET' | 'DELETE';

export interface KVEntry {
    key: string;
    value: string;
    version: number;
}

export interface WALRecord {
    op: KVOperation;
    key: string;
    value?: string;
    timestamp: number;
}

export interface Snapshot {
    entries: Record<string, KVEntry>;
    takenAt: number;
}

export interface Node {
    id: string;
    angle: number;
    role?: "primary" | "replica" | "idle";
}