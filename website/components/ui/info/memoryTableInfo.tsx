import { InfoTooltip } from "../info";

export function MemtableInfo() {
    return (
        <InfoTooltip
            term="Memory Table (MemTable)"
            description="A fast in-memory space where new data is kept while the server is running. When it grows too big, the data is saved to disk and a fresh memory table is started."
        >
            Memory Table
        </InfoTooltip>
    );
}
