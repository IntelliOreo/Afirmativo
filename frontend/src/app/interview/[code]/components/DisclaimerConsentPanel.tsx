"use client";

import type { RefObject } from "react";
import { Alert } from "@components/Alert";
import { Button } from "@components/Button";
import { Card } from "@components/Card";
import type { Lang } from "@/lib/language";
import { getInterviewMessages } from "../messages/interviewMessages";
import type { DisclaimerBlock } from "../viewTypes";

interface DisclaimerConsentPanelProps {
  lang: Lang;
  consentBlocks: DisclaimerBlock[];
  warningAlertText: string;
  hasReachedDisclaimerBottom: boolean;
  disclaimerScrollRef: RefObject<HTMLDivElement | null>;
  onDisclaimerScroll: () => void;
  onAgreeAndContinue: () => void | Promise<void>;
}

export function DisclaimerConsentPanel({
  lang,
  consentBlocks,
  warningAlertText,
  hasReachedDisclaimerBottom,
  disclaimerScrollRef,
  onDisclaimerScroll,
  onAgreeAndContinue,
}: DisclaimerConsentPanelProps) {
  const t = getInterviewMessages(lang).disclaimer;

  return (
    <>
      <Card className="mb-6">
        <div
          ref={disclaimerScrollRef}
          onScroll={onDisclaimerScroll}
          className="max-h-72 overflow-y-auto pr-1"
        >
          <div className="space-y-4 text-base font-normal text-primary-darkest leading-relaxed">
            {consentBlocks.map((block, index) =>
              block.type === "paragraph" ? (
                <p key={`p-${index}`} className="whitespace-pre-line">
                  {block.text}
                </p>
              ) : (
                <ul key={`l-${index}`} className="list-disc list-inside space-y-2">
                  {block.items.map((item, itemIndex) => (
                    <li key={`li-${index}-${itemIndex}`}>{item}</li>
                  ))}
                </ul>
              ),
            )}
          </div>
        </div>
      </Card>

      <Alert variant="warning" className="mb-6">
        {warningAlertText}
      </Alert>

      {!hasReachedDisclaimerBottom && (
        <p className="text-sm text-primary-darkest mb-4">
          {t.scrollToContinue}
        </p>
      )}

      <Button
        fullWidth
        disabled={!hasReachedDisclaimerBottom}
        onClick={() => { void onAgreeAndContinue(); }}
      >
        {t.agree}
      </Button>
    </>
  );
}
