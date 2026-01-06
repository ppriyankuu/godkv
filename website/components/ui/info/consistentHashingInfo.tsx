import { InfoTooltip } from "../info";

export function ConsistentHashingInfo() {
    return (
        <InfoTooltip
            term="Consistent Hashing"
            description="A technique to distribute keys across nodes so that when nodes are added or removed, only a small portion of keys need to be remapped—unlike naive hashing, which would reshuffle everything."
        >
            consistent hashing
        </InfoTooltip>
    );
}