import { InfoTooltip } from "../info";

export function ReplicationInfo() {
    return (
        <InfoTooltip
            term="Replication"
            description="Storing copies of data on multiple nodes to ensure availability and durability. If one node fails, others can still serve the data."
        >
            replication
        </InfoTooltip>
    );
}