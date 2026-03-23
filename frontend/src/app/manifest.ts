import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Affirmative Interview Simulator | Simulador de Entrevista Afirmativa",
    short_name: "Afirmativo",
    description: "Bilingual asylum interview practice with AI-guided English and Spanish flows.",
    start_url: "/",
    scope: "/",
    display: "standalone",
    background_color: "#f0f0f0",
    theme_color: "#1A4480",
    icons: [
      {
        src: "/icon",
        sizes: "512x512",
        type: "image/png",
      },
      {
        src: "/apple-icon",
        sizes: "180x180",
        type: "image/png",
      },
    ],
  };
}
