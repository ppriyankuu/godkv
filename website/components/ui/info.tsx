"use client";

import { useState, useRef, useEffect } from "react";

interface InfoTooltipProps {
    term: string;
    description: string;
    children?: React.ReactNode;
}

export function InfoTooltip({ term, description, children }: InfoTooltipProps) {
    const [isVisible, setIsVisible] = useState(false);
    const [position, setPosition] = useState({ top: 0, left: 0 });
    const triggerRef = useRef<HTMLSpanElement>(null);
    const tooltipRef = useRef<HTMLSpanElement>(null);

    const [isTouchDevice, setIsTouchDevice] = useState(false);

    useEffect(() => {
        setIsTouchDevice('ontouchstart' in window || navigator.maxTouchPoints > 0);
    }, []);

    useEffect(() => {
        if (isVisible && triggerRef.current) {
            const rect = triggerRef.current.getBoundingClientRect();
            setPosition({
                top: rect.bottom + window.scrollY + 8,
                left: rect.left + window.scrollX,
            });
        }
    }, [isVisible]);

    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (
                tooltipRef.current &&
                !tooltipRef.current.contains(event.target as Node) &&
                triggerRef.current &&
                !triggerRef.current.contains(event.target as Node)
            ) {
                setIsVisible(false);
            }
        };

        const handleEscape = (event: KeyboardEvent) => {
            if (event.key === "Escape") {
                setIsVisible(false);
            }
        };

        if (isVisible) {
            document.addEventListener("mousedown", handleClickOutside);
            document.addEventListener("keydown", handleEscape);
        }

        return () => {
            document.removeEventListener("mousedown", handleClickOutside);
            document.removeEventListener("keydown", handleEscape);
        };
    }, [isVisible]);

    const handleInteraction = () => {
        setIsVisible(!isVisible);
    };

    const eventHandlers = isTouchDevice
        ? {
            onClick: handleInteraction,
        }
        : {
            onMouseEnter: () => setIsVisible(true),
            onMouseLeave: () => setIsVisible(false),
            onFocus: () => setIsVisible(true),
            onBlur: () => setIsVisible(false),
        };

    return (
        <span className="inline-block relative">
            <span
                ref={triggerRef}
                onClick={isTouchDevice ? handleInteraction : undefined}
                {...eventHandlers}
                className="underline decoration-dotted underline-offset-4 cursor-help text-blue-300 hover:text-blue-200 transition-colors"
                aria-describedby={`tooltip-${term}`}
                role="button"
                tabIndex={0}
            >
                {children || term}
            </span>

            {isVisible && (
                <span
                    ref={tooltipRef}
                    id={`tooltip-${term}`}
                    className="fixed z-50 w-64 p-3 text-sm bg-zinc-800 border border-zinc-700 rounded-lg shadow-lg"
                    style={{
                        top: `${position.top}px`,
                        left: `${Math.min(position.left, window.innerWidth - 272)}px`,
                    }}
                    role="tooltip"
                >
                    <span className="font-semibold text-zinc-100 mb-1 block">{term}</span>
                    <span className="text-zinc-300 block">{description}</span>
                    <span className="absolute -top-1 left-4 w-2 h-2 bg-zinc-800 border-t border-l border-zinc-700 transform rotate-45 block"></span>
                </span>
            )}
        </span>
    );
}