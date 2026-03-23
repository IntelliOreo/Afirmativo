import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";
import { NavHeader } from "./NavHeader";

vi.mock("next/link", () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>
      {children}
    </a>
  ),
}));

describe("NavHeader", () => {
  it("keeps the shared language toggle pinned on the right without collapsing its width", () => {
    render(<NavHeader lang="es" onToggleLang={vi.fn()} />);

    const wrapper = screen.getByRole("banner").firstElementChild;
    const toggle = screen.getByRole("button", { name: "English" });
    const titleLink = screen.getByRole("link", { name: "Simulador de Entrevista Afirmativa" });

    expect(wrapper).toHaveClass("justify-between");
    expect(wrapper).not.toHaveClass("flex-col");
    expect(titleLink).toHaveClass("flex-1");
    expect(toggle).toHaveClass("shrink-0");
    expect(toggle).toHaveClass("text-lg");
    expect(toggle).toHaveClass("font-medium");
    expect(toggle).not.toHaveClass("text-base");
  });

  it("omits the language toggle when no toggle handler is provided", () => {
    render(<NavHeader lang="es" />);

    expect(screen.queryByRole("button", { name: "English" })).not.toBeInTheDocument();
  });
});
