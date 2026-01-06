import { Card } from "@/components/ui/card";

export default function HomePage() {
  return (
    <div className="space-y-10 mt-8 md:mt-48">
      <section className="text-center space-y-4">
        <h1 className="text-4xl font-bold text-zinc-300 tracking-tight sm:text-5xl">
          goDKV
        </h1>
        <p className="text-lg text-zinc-400 max-w-3xl mx-auto leading-relaxed">
          A visual simulation to explain how my distributed key-value store works—built entirely in Go.
          No real database, just interactive demos to help you (and me!) understand the core ideas.
        </p>
        <p className="text-sm text-zinc-500">
          <a
            href="https://github.com/ppriyankuu/godkv"
            target="_blank"
            rel="noopener noreferrer"
            className="text-zinc-400 hover:text-zinc-300 underline"
          >
            View the backend code on GitHub {'->'}
          </a>
        </p>
      </section>

      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <Card
          title="Single Node KV"
          description="Write-Ahead Logging, in-memory store, snapshots, and crash recovery."
          href="/single"
        />
        <Card
          title="Distributed KV"
          description="Consistent hashing, quorum reads/writes, and replication across nodes."
          href="/distributed"
        />
        <Card
          title="System Architecture"
          description="See how WAL, hash ring, and quorum tie together in the full design."
          href="/architecture"
        />
      </div>

      <div className="text-center my-4">
        <p className="text-sm text-zinc-500">
          Click any section to explore live visualizations and try operations yourself.
        </p>
      </div>
    </div>
  );
}
