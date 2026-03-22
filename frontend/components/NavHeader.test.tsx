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
  it("renders the shared language toggle with the larger text size", () => {
    render(<NavHeader lang="es" onToggleLang={vi.fn()} />);

    const toggle = screen.getByRole("button", { name: "English" });
    expect(toggle).toHaveClass("text-lg");
    expect(toggle).toHaveClass("font-medium");
    expect(toggle).not.toHaveClass("text-base");
  });

  it("omits the language toggle when no toggle handler is provided", () => {
    render(<NavHeader lang="es" />);

    expect(screen.queryByRole("button", { name: "English" })).not.toBeInTheDocument();
  });
});
