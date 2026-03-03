export function Footer() {
  return (
    <footer className="bg-base-lightest border-t border-base-lighter mt-auto">
      <div className="max-w-5xl mx-auto px-4 py-8 text-sm text-gray-600">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <p className="font-semibold text-primary-darkest mb-1">
              Affirmative Interview Simulator
            </p>
            <p>Simulador de Entrevista Afirmativa</p>
          </div>
          <div className="text-left sm:text-right">
            <p>
              This tool is for preparation purposes only.
            </p>
            <p>
              Esta herramienta es solo para fines de preparación.
            </p>
          </div>
        </div>
      </div>
    </footer>
  );
}
