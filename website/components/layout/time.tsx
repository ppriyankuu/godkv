'use client';

import { useEffect, useState } from 'react';

export function ClientTime({ timestamp }: { timestamp: number | Date }) {
    const [value, setValue] = useState<string>("");

    useEffect(() => {
        const date = timestamp instanceof Date ? timestamp : new Date(timestamp);
        setValue(
            date.toLocaleTimeString('en-US', {
                hour12: false,
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
            })
        );
    }, [timestamp]);

    return <>{value}</>;
}
