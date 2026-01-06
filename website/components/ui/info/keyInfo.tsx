import { InfoTooltip } from "../info";

export function KeyInfo() {
    return (
        <InfoTooltip
            term="Key"
            description="A key is the identifier used to store and retrieve data (for example: user:123). In a distributed system, the key is hashed to a position on the hash ring, which determines which node(s) are responsible for storing its value."
        >
            key
        </InfoTooltip>
    );
}
