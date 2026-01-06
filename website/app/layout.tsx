// src/app/layout.tsx
import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "@/app/globals.css";
import { Container } from "@/components/layout/container";
import { Navbar } from "@/components/layout/navbar";

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "goDKV",
  description: "Interactive visualization of single-node and distributed key-value systems.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang=" " className="h-full scroll-smooth">
      <body className={`${inter.className} h-full`}>
        <div className="flex min-h-screen flex-col bg-zinc-950">
          <Navbar />
          <main className="grow">
            <Container>{children}</Container>
          </main>
        </div>
      </body>
    </html>
  );
}
