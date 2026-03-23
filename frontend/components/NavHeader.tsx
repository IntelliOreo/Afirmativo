import Link from "next/link";

interface NavHeaderProps {
  lang?: "es" | "en";
  onToggleLang?: () => void;
}

export function NavHeader({ lang = "es", onToggleLang }: NavHeaderProps) {
  return (
    <header className="bg-primary-dark text-white">
      <div className="max-w-5xl mx-auto px-4 py-3 flex items-start justify-between gap-3 sm:items-center">
        <Link href="/" className="flex-1 min-w-0 text-white no-underline hover:underline">
          <span className="block font-bold text-base leading-tight break-words sm:text-lg">
            {lang === "es"
              ? "Simulador de Entrevista Afirmativa"
              : "Affirmative Interview Simulator"}
          </span>
        </Link>
        {onToggleLang && (
          <button
            onClick={onToggleLang}
            className="shrink-0 self-start text-lg font-medium text-white underline hover:no-underline bg-transparent border-0 cursor-pointer sm:self-auto"
          >
            {lang === "es" ? "English" : "Español"}
          </button>
        )}
      </div>
    </header>
  );
}
