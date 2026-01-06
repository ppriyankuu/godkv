import { InfoTooltip } from "../info";

export function HashRingInfo() {
    return (
        <InfoTooltip
            term="Hash Ring"
            description="A circular namespace (often 0 to 2^32 or 0–360°) where both keys and nodes are placed using a hash function. Keys are assigned to the next node(s) clockwise on the ring."
        >
            hash ring
        </InfoTooltip>
    );
}