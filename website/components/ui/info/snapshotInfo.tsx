import { InfoTooltip } from "../info";

export function SnapshotInfo() {
    return (
        <InfoTooltip
            term="Snapshot"
            description="A saved picture of the database at a moment in time. It helps the system start faster after a crash and keeps old log files from growing too large."
        >
            Snapshot
        </InfoTooltip>
    );
}
