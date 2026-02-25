import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        // USWDS-aligned color palette
        "primary-darkest": "#1B1B1B",
        "primary-dark": "#1A4480",
        primary: "#005EA2",
        "primary-light": "#73B3E7",
        "accent-warm": "#FA9441",
        "accent-cool": "#00BDE3",
        error: "#D54309",
        success: "#00A91C",
        "base-lightest": "#F0F0F0",
        "base-lighter": "#DFE1E2",
      },
      fontFamily: {
        sans: ["var(--font-source-sans)", "system-ui", "sans-serif"],
      },
      lineHeight: {
        body: "1.625",
      },
      borderRadius: {
        btn: "4px",
      },
    },
  },
  plugins: [],
};

export default config;
