import Link from "next/link";

interface CardProps {
    title: string;
    description: string;
    href?: string;
}

export function Card({ title, description, href }: CardProps) {
    const Wrapper = href ? Link : "div";

    return (
        <Wrapper
            href={href as any}
            className="block rounded-lg border border-zinc-800 bg-zinc-900 p-5 hover:border-zinc-600 transition"
        >
            <h3 className="font-semibold mb-1 text-zinc-300">{title}</h3>
            <p className="text-sm text-zinc-400">{description}</p>
        </Wrapper>
    );
}
