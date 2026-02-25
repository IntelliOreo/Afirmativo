import Link from "next/link";

interface NavHeaderProps {
  lang?: "es" | "en";
  onToggleLang?: () => void;
}

export function NavHeader({ lang = "es", onToggleLang }: NavHeaderProps) {
  return (
    <header className="bg-primary-dark text-white">
      <div className="max-w-5xl mx-auto px-4 py-3 flex items-center justify-between">
        <Link href="/" className="text-white no-underline hover:underline">
          <span className="font-bold text-lg">
            {lang === "es"
              ? "Simulador de Entrevista Afirmativa"
              : "Affirmative Interview Simulator"}
          </span>
        </Link>
        {onToggleLang && (
          <button
            onClick={onToggleLang}
            className="text-sm text-white underline hover:no-underline bg-transparent border-0 cursor-pointer"
          >
            {lang === "es" ? "English" : "Español"}
          </button>
        )}
      </div>
    </header>
  );
}
