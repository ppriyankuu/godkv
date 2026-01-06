'use client';

import Link from "next/link";
import { usePathname } from "next/navigation";

const navItems = [
    { name: "Single Node", short: "sng", href: "/single" },
    { name: "Distributed", short: "dis", href: "/distributed" },
    { name: "Architecture", short: "arch", href: "/architecture" },
];

export function Navbar() {
    const pathname = usePathname();

    return (
        <nav className="border-b border-zinc-800 bg-zinc-900 sticky top-0 z-10">
            <div className="container mx-auto px-6 md:px-4 py-3 flex items-center justify-between">
                {/* Logo */}
                <Link href="/" className="font-bold text-lg text-zinc-300 shrink-0">
                    goDKV
                </Link>

                {/* Nav Links */}
                <div className="flex items-center space-x-2 md:space-x-4 ml-4 overflow-x-auto hide-scrollbar">
                    {navItems.map((item) => (
                        <Link
                            key={item.href}
                            href={item.href}
                            className="whitespace-nowrap px-2 py-1 rounded text-sm shrink-0 text-white font-medium bg-zinc-800"
                        >
                            <span className="hidden sm:inline">{item.name}</span>
                            <span className="sm:hidden">{item.short}</span>
                        </Link>
                    ))}
                </div>
            </div>
        </nav>
    );
}