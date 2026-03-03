import Link from "next/link";

interface NavHeaderProps {
  lang?: "es" | "en";
  onToggleLang?: () => void;
}

export function NavHeader({ lang = "es", onToggleLang }: NavHeaderProps) {
  return (
    <header className="bg-primary-dark text-white">
      <div className="max-w-5xl mx-auto px-4 py-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <Link href="/" className="text-white no-underline hover:underline min-w-0">
          <span className="block font-bold text-base leading-tight break-words sm:text-lg">
            {lang === "es"
              ? "Simulador de Entrevista Afirmativa"
              : "Affirmative Interview Simulator"}
          </span>
        </Link>
        {onToggleLang && (
          <button
            onClick={onToggleLang}
            className="self-start text-sm text-white underline hover:no-underline bg-transparent border-0 cursor-pointer sm:self-auto"
          >
            {lang === "es" ? "English" : "Español"}
          </button>
        )}
      </div>
    </header>
  );
}
