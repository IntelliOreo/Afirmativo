import type { Lang } from "@/lib/language";

export type WaitingStatusPhase = "startup" | "question" | "report";

export const WAITING_STATUS_ROTATION_MS = 4000;
export const WAITING_STATUS_INITIAL_DELAY_MS = 2000;

const WAITING_STATUS_EN = [
  "The interviewer is flipping through your case files... It's a lot of reading... They're doing their best...",
  "Your response is being carefully considered... Good answers have a habit of complicating things — in the best way...",
  "The interviewer reaches for their water... It's their third glass... Hydration is a lifestyle...",
  "Your file is being reviewed, page by page... Whoever put it together really committed to the details...",
  "The interviewer scribbles a note... Their handwriting is truly something... Only they know what it says...",
  "A brief pause to double-check a few things... The interviewer has a reputation for catching the small stuff...",
  "The interviewer leans back, stares at the ceiling for a moment, and thinks... It's part of their process...",
  "Somewhere nearby, a printer is whirring... More documents incoming... There are always more documents...",
  "Your response landed and now the gears are turning... Good things take a moment to process...",
  "The interviewer takes a slow sip of water and exhales... This is their thinking ritual... It usually works...",
  "Case files, sticky notes, and a coffee that went cold an hour ago... The interviewer's desk has seen things...",
  "A quiet moment in the room... The interviewer doesn't mind the silence... You might find it a little awkward...",
  "The interviewer flips back a page... Something caught their eye — probably nothing... Probably...",
  "They've heard a lot of stories in this room... Yours is one of the more interesting ones... That's not nothing...",
  "Almost time for the next question... The interviewer is just finishing a thought — and possibly that glass of water...",
] as const;

const WAITING_STATUS_ES = [
  "El entrevistador está revisando tu expediente... Hay mucho que leer... Está haciendo su mejor esfuerzo...",
  "Tu respuesta está siendo considerada con cuidado... Las buenas respuestas tienen la costumbre de complicar las cosas — de la mejor manera...",
  "El entrevistador toma su vaso de agua... Es el tercero... La hidratación es un estilo de vida...",
  "Tu expediente está siendo revisado, página por página... Quien lo preparó realmente se comprometió con los detalles...",
  "El entrevistador garabatea una nota... Su letra es toda una obra... Solo él sabe lo que dice...",
  "Una breve pausa para verificar algunas cosas... El entrevistador tiene fama de notar los pequeños detalles...",
  "El entrevistador se recuesta, mira el techo un momento y piensa... Es parte de su proceso...",
  "En algún lugar cercano, una impresora zumba... Más documentos en camino... Siempre hay más documentos...",
  "Tu respuesta fue recibida y los engranajes están girando... Las cosas buenas toman un momento para procesarse...",
  "El entrevistador da un lento sorbo de agua y exhala... Es su ritual para pensar... Generalmente funciona...",
  "Expedientes, notas adhesivas y un café que se enfrió hace una hora... El escritorio del entrevistador ha visto cosas...",
  "Un momento de silencio en la sala... Al entrevistador no le molesta el silencio... A ti quizás sí un poco...",
  "El entrevistador regresa una página... Algo llamó su atención — probablemente nada... Probablemente...",
  "Han escuchado muchas historias en esta sala... La tuya es una de las más interesantes... Eso no es poco...",
  "Casi es momento para la siguiente pregunta... El entrevistador está terminando un pensamiento — y posiblemente ese vaso de agua...",
] as const;

const byPhase = (messages: readonly string[]): Record<WaitingStatusPhase, readonly string[]> => ({
  startup: messages,
  question: messages,
  report: messages,
});

export const WAITING_STATUS_MESSAGES: Record<Lang, Record<WaitingStatusPhase, readonly string[]>> = {
  en: byPhase(WAITING_STATUS_EN),
  es: byPhase(WAITING_STATUS_ES),
};
