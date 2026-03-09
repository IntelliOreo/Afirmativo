"use client";

import { useMemo, useState } from "react";

import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import { Footer } from "@components/Footer";
import { Input } from "@components/Input";
import { NavHeader } from "@components/NavHeader";

type CleanupResult = {
  hours: number;
  cutoff: string;
  deleted: {
    answers: number;
    interview_events: number;
    question_areas: number;
    reports: number;
    sessions: number;
  };
  total_deleted: number;
};

type ErrorResult = {
  error?: string;
  code?: string;
};

function formatCutoff(raw: string): string {
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  return d.toLocaleString();
}

export function AdminPageClient() {
  const [hours, setHours] = useState("24");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<CleanupResult | null>(null);

  const totalDeleted = useMemo(() => result?.total_deleted ?? 0, [result]);

  async function runCleanup() {
    setError("");

    const trimmed = hours.trim();
    const payload: { hours?: number } = {};
    if (trimmed !== "") {
      const parsed = Number(trimmed);
      if (!Number.isInteger(parsed) || parsed <= 0) {
        setError("Horas inválidas. Debe ser un número entero mayor que 0. / Invalid hours. Must be an integer greater than 0.");
        return;
      }
      payload.hours = parsed;
    }

    setLoading(true);
    try {
      const res = await fetch("/api/admin/cleanup-db", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });

      const contentType = res.headers.get("content-type") ?? "";
      let data: CleanupResult | ErrorResult | null = null;
      if (contentType.includes("application/json")) {
        data = (await res.json()) as CleanupResult | ErrorResult;
      }

      if (!res.ok) {
        const msg =
          data && "error" in data && data.error
            ? data.error
            : "No se pudo ejecutar la limpieza. / Failed to run cleanup.";
        setError(msg);
        return;
      }

      setResult(data as CleanupResult);
    } catch {
      setError("Error de red al ejecutar limpieza. / Network error while running cleanup.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex flex-col min-h-screen">
      <NavHeader lang="es" />

      <main className="flex-1 bg-base-lightest">
        <div className="max-w-3xl mx-auto px-4 py-8 sm:py-12">
          <h1 className="text-2xl sm:text-3xl font-bold text-primary-dark mb-2">
            Admin: Limpieza de Base de Datos / Database Cleanup
          </h1>
          <p className="text-primary-darkest mb-6">
            Herramienta solo para desarrollo. / Development-only tool.
          </p>

          <Card className="mb-6">
            <div className="space-y-4">
              <Input
                type="number"
                min={1}
                step={1}
                label="Horas de antigüedad / Age in hours"
                hint="Si se deja vacío usa 24 horas por defecto. / If left blank, defaults to 24 hours."
                value={hours}
                onChange={(e) => setHours(e.target.value)}
              />
              <Button onClick={runCleanup} disabled={loading} fullWidth>
                {loading
                  ? "Ejecutando limpieza... / Running cleanup..."
                  : "Ejecutar limpieza / Run cleanup"}
              </Button>
            </div>
          </Card>

          {error && (
            <Alert variant="error" className="mb-6">
              {error}
            </Alert>
          )}

          {result && (
            <Card>
              <h2 className="text-xl font-bold text-primary-dark mb-3">
                Resultado / Result
              </h2>
              <p className="text-sm text-gray-700 mb-1">
                Horas usadas / Hours used: <span className="font-semibold">{result.hours}</span>
              </p>
              <p className="text-sm text-gray-700 mb-4">
                Fecha límite / Cutoff:{" "}
                <span className="font-semibold">{formatCutoff(result.cutoff)}</span>
              </p>

              <div className="border border-base-lighter rounded overflow-hidden mb-4">
                <table className="w-full text-sm">
                  <thead className="bg-base-lightest">
                    <tr>
                      <th className="text-left p-2 font-semibold text-primary-dark">Tabla / Table</th>
                      <th className="text-right p-2 font-semibold text-primary-dark">Eliminados / Deleted</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr className="border-t border-base-lighter">
                      <td className="p-2">answers</td>
                      <td className="p-2 text-right">{result.deleted.answers}</td>
                    </tr>
                    <tr className="border-t border-base-lighter">
                      <td className="p-2">interview_events</td>
                      <td className="p-2 text-right">{result.deleted.interview_events}</td>
                    </tr>
                    <tr className="border-t border-base-lighter">
                      <td className="p-2">question_areas</td>
                      <td className="p-2 text-right">{result.deleted.question_areas}</td>
                    </tr>
                    <tr className="border-t border-base-lighter">
                      <td className="p-2">reports</td>
                      <td className="p-2 text-right">{result.deleted.reports}</td>
                    </tr>
                    <tr className="border-t border-base-lighter">
                      <td className="p-2">sessions</td>
                      <td className="p-2 text-right">{result.deleted.sessions}</td>
                    </tr>
                  </tbody>
                </table>
              </div>

              <Alert variant="success">
                Total eliminados / Total deleted: <span className="font-semibold">{totalDeleted}</span>
              </Alert>
            </Card>
          )}
        </div>
      </main>

      <Footer />
    </div>
  );
}
