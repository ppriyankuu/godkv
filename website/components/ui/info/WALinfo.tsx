import { InfoTooltip } from "../info";

export function WALInfo() {
    return (
        <InfoTooltip
            term="Write-Ahead Log (WAL)"
            description="A safety log on disk. Every change is written here first so data is not lost if the server crashes. On restart, the system reads this log to rebuild what was last written."
        >
            Write-Ahead Log (WAL)
        </InfoTooltip>
    );
}
