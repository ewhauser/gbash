"use client";

import benchmarkData from "@/content/performance/filesystem-benchmark-data.json";

interface Stats {
  min_nanos: number;
  median_nanos: number;
  p95_nanos: number;
}

interface MachineInfo {
  model: string;
  chip: string;
  cores: string;
  memory: string;
  os: string;
  go_version: string;
}

interface FixtureSummary {
  file_count: number;
  total_bytes: number;
}

interface BackendInfo {
  name: string;
  label: string;
  description: string;
  experimental?: boolean;
}

interface ScenarioResult {
  backend: string;
  success_count: number;
  failure_count: number;
  search_mode?: string;
  stats: Stats;
}

interface Scenario {
  name: string;
  description: string;
  results: ScenarioResult[];
}

interface BenchmarkReport {
  generated_at: string;
  runs: number;
  machine: MachineInfo;
  fixture: FixtureSummary;
  backends: BackendInfo[];
  scenarios: Scenario[];
}

const data = benchmarkData as BenchmarkReport;

function formatNanos(nanos: number): string {
  if (nanos >= 1_000_000_000) return `${(nanos / 1_000_000_000).toFixed(2)}s`;
  if (nanos >= 1_000_000) return `${(nanos / 1_000_000).toFixed(1)}ms`;
  if (nanos >= 1_000) return `${(nanos / 1_000).toFixed(1)}µs`;
  return `${nanos}ns`;
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024 * 1024)
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GiB`;
  if (bytes >= 1024 * 1024)
    return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${bytes} B`;
}

const backendByName = new Map(data.backends.map((backend) => [backend.name, backend]));

function MachineTable() {
  const m = data.machine;
  return (
    <div className="mb-8">
      <h3 className="text-lg font-semibold text-[var(--fg-primary)]">
        Filesystem Test Environment
      </h3>
      <div className="overflow-x-auto">
        <table>
          <tbody>
            {[
              ["Machine", m.model],
              ["Chip", m.chip],
              ["Cores", m.cores],
              ["Memory", m.memory],
              ["OS", m.os],
              ["Go", m.go_version],
              ["Runs per scenario", `${data.runs}`],
              [
                "Fixture",
                `${data.fixture.file_count} files, ${formatBytes(data.fixture.total_bytes)}`,
              ],
              [
                "Generated",
                new Date(data.generated_at).toLocaleDateString("en-US", {
                  year: "numeric",
                  month: "long",
                  day: "numeric",
                }),
              ],
            ].map(([label, value]) => (
              <tr key={label}>
                <td className="font-medium">{label}</td>
                <td>{value}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function BackendTable() {
  return (
    <div className="mb-8">
      <h3 className="text-lg font-semibold text-[var(--fg-primary)]">Backends</h3>
      <p className="text-sm text-[var(--fg-secondary)] mt-1 mb-3">
        `memory` and `overlay` are core modes. `sqlite` and `fts` are experimental
        example-backed filesystems.
      </p>
      <div className="overflow-x-auto">
        <table>
          <thead>
            <tr>
              <th>Backend</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {data.backends.map((backend) => (
              <tr key={backend.name}>
                <td>
                  <div>{backend.label}</div>
                  {backend.experimental && (
                    <div className="text-xs text-[var(--fg-secondary)] mt-1">
                      Experimental example backend
                    </div>
                  )}
                </td>
                <td>{backend.description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ScenarioTable({ scenario }: { scenario: Scenario }) {
  return (
    <div className="mb-8">
      <h3 className="text-lg font-semibold text-[var(--fg-primary)]">
        {scenario.name.replace(/_/g, " ")}
      </h3>
      <p className="text-sm text-[var(--fg-secondary)] mt-1 mb-3">
        {scenario.description}
      </p>
      <div className="overflow-x-auto">
        <table>
          <thead>
            <tr>
              <th>Backend</th>
              <th>Min</th>
              <th>Median</th>
              <th>p95</th>
            </tr>
          </thead>
          <tbody>
            {scenario.results.map((result) => {
              const backend = backendByName.get(result.backend);
              return (
                <tr key={result.backend}>
                  <td>
                    <div>{backend?.label ?? result.backend}</div>
                    {result.search_mode && (
                      <div className="text-xs text-[var(--fg-secondary)] mt-1">
                        Search mode: {result.search_mode}
                      </div>
                    )}
                  </td>
                  <td>{formatNanos(result.stats.min_nanos)}</td>
                  <td>{formatNanos(result.stats.median_nanos)}</td>
                  <td>{formatNanos(result.stats.p95_nanos)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default function FilesystemBenchmarkChart() {
  return (
    <div>
      <MachineTable />
      <BackendTable />
      {data.scenarios.map((scenario) => (
        <ScenarioTable key={scenario.name} scenario={scenario} />
      ))}
    </div>
  );
}
