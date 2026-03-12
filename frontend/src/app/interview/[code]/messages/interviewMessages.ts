import type { Lang } from "@/lib/language";
import { VOICE_MAX_SECONDS } from "../constants";
import type {
  CompletionSource,
  MicWarmState,
  MicrophoneWarmupDialogMode,
  MicrophoneWarmupDialogState,
  ReportErrorState,
  VoiceFeedback,
  VoiceRecorderState,
} from "../viewTypes";

type AlertVariant = "info" | "warning" | "error" | "success";

type InterviewCopy = {
  guard: {
    title: string;
    body: string;
    recoverButton: string;
  };
  error: {
    prefix: string;
    pendingAnswerBody: string;
    retryPendingAnswer: string;
    reloadPage: string;
    reloadRecoveryBody: string;
    recoverWithPin: string;
    backHome: string;
  };
  finalSubmit: {
    title: string;
    body: string;
  };
  disclaimer: {
    scrollToContinue: string;
    agree: string;
  };
  progress: {
    timeRemaining: string;
  };
  inputMode: {
    text: string;
    voice: string;
  };
  textAnswer: {
    submitWindowLabel: string;
    finalDraft: string;
    answerInSelectedLanguage: string;
    placeholder: string;
    characters: string;
    timeExpired: string;
    submit: string;
  };
  page: {
    loading: string;
    processingAnswer: string;
    backHome: string;
    understandAnswer: string;
    stopRecordingBeforeSwitch: string;
    discardVoiceSwitchConfirm: string;
  };
  report: {
    completedTitle: string;
    alreadyCompletedBody: string;
    finishedBody: string;
    generate: string;
    loading: string;
    generating: string;
    checkAgain: string;
    errorPrefix: string;
    tryAgain: string;
    print: string;
    printHint: string;
    summaryTitle: string;
    questionsMinutes: (questionCount: number, durationMinutes: number) => string;
    clarityTitle: string;
    developTitle: string;
    recommendationTitle: string;
    fullAssessmentTitle: string;
    noItems: string;
  };
  voice: {
    submitWindowLabel: string;
    reviewWarning: string;
    playbackAria: string;
    reviewableTranscript: string;
    transcriptPlaceholder: string;
    rerecord: string;
    discard: string;
    complete: string;
    timeExpired: string;
    reviewTranscript: string;
    reviewingTranscript: string;
    submitAnswer: string;
    audioReadyLabel: string;
    warningBeforeLimit: (remainingSeconds: number, limitLabel: string) => string;
  };
  microphoneDialog: {
    dismissReconnect: string;
    dismissInitial: string;
  };
  timeoutDialog: {
    eyebrow: string;
    title: string;
    body: string;
    status: string;
    interruptedTitle: string;
    interruptedBody: string;
    interruptedStatus: string;
    transcribingTitle: string;
    transcribingBody: string;
    transcribingStatus: string;
  };
};

const INTERVIEW_MESSAGES = {
  en: {
    guard: {
      title: "Session not found",
      body: "No active session found in this browser. If you have a session code and PIN, you can recover your access.",
      recoverButton: "Recover session",
    },
    error: {
      prefix: "Error:",
      pendingAnswerBody: "A pending answer is still saved. Retry sending it or reload the page to continue.",
      retryPendingAnswer: "Retry send",
      reloadPage: "Reload page",
      reloadRecoveryBody: "This session got out of sync. Reload this page to fetch the latest interview state.",
      recoverWithPin: "Recover session with PIN",
      backHome: "Back to home",
    },
    finalSubmit: {
      title: "Finalizing your answer",
      body: "Editing is locked while we finish transcribing and submitting your answer.",
    },
    disclaimer: {
      scrollToContinue: "Scroll to the bottom of the disclaimer to continue.",
      agree: "I understand",
    },
    progress: {
      timeRemaining: "Maximum interview time remaining",
    },
    inputMode: {
      text: "Text input",
      voice: "Voice input",
    },
    textAnswer: {
      submitWindowLabel: "Submit this answer in",
      finalDraft: "Final draft",
      answerInSelectedLanguage: "Please answer in your selected language",
      placeholder: "Type your answer here...",
      characters: "Characters",
      timeExpired: "Time is up - please submit your answer now.",
      submit: "Submit answer",
    },
    page: {
      loading: "Loading...",
      processingAnswer: "Processing answer",
      backHome: "Back to home",
      understandAnswer: "I understand",
      stopRecordingBeforeSwitch: "Stop recording before switching modes.",
      discardVoiceSwitchConfirm: "You have audio that has not been reviewed yet. Discard it and switch modes?",
    },
    report: {
      completedTitle: "Interview completed",
      alreadyCompletedBody: "This interview was already completed. You can generate your report here.",
      finishedBody: "All criteria were evaluated. Generate your report when you are ready.",
      generate: "Generate report",
      loading: "Loading report",
      generating: "Generating report",
      checkAgain: "Check again",
      errorPrefix: "Error:",
      tryAgain: "Try again",
      print: "Print / Save as PDF",
      printHint: "On mobile: tap Print, then Save as PDF.",
      summaryTitle: "Preparation feedback summary",
      questionsMinutes: (questionCount: number, durationMinutes: number) => `${questionCount} questions - ${durationMinutes} minutes`,
      clarityTitle: "Areas of clarity",
      developTitle: "Areas to develop further",
      recommendationTitle: "Recommendation",
      fullAssessmentTitle: "Full assessment",
      noItems: "No items to display.",
    },
    voice: {
      submitWindowLabel: "Submit this answer in",
      reviewWarning: "Stop soon to leave time to review before submit.",
      playbackAria: "Play recorded audio",
      reviewableTranscript: "Reviewable transcript",
      transcriptPlaceholder: "Edit the transcript here...",
      rerecord: "Re-record",
      discard: "Discard",
      complete: "Complete",
      timeExpired: "Time is up - please submit your answer now.",
      reviewTranscript: "Review transcript",
      reviewingTranscript: "Reviewing transcript...",
      submitAnswer: "Submit answer",
      audioReadyLabel: "Audio ready",
      warningBeforeLimit: (remainingSeconds: number, limitLabel: string) => `${remainingSeconds}s remain before the limit of ${limitLabel}.`,
    },
    microphoneDialog: {
      dismissReconnect: "Close",
      dismissInitial: "Not now",
    },
    timeoutDialog: {
      eyebrow: "Answer window ended",
      title: "Time is up",
      body: "Submit now to continue to the next question.",
      status: "Your answer is ready to submit.",
      interruptedTitle: "This answer window already ended",
      interruptedBody: "This page reopened after the answer window ended. Submit now to continue.",
      interruptedStatus: "Your previous answer can no longer be edited after the interruption.",
      transcribingTitle: "Transcribing your recording",
      transcribingBody: "We are turning your recording into text so you can submit this answer.",
      transcribingStatus: "Transcription in progress.",
    },
  },
  es: {
    guard: {
      title: "Sesion no encontrada",
      body: "No se encontro una sesion activa en este navegador. Si tiene un codigo de sesion y PIN, puede recuperar su acceso.",
      recoverButton: "Recuperar sesion",
    },
    error: {
      prefix: "Error:",
      pendingAnswerBody: "Tiene una respuesta pendiente guardada. Intente reenviarla o recargue la pagina para continuar.",
      retryPendingAnswer: "Reintentar envio",
      reloadPage: "Recargar pagina",
      reloadRecoveryBody: "Esta sesion se desincronizo. Recargue esta pagina para obtener el estado mas reciente de la entrevista.",
      recoverWithPin: "Recuperar sesion con PIN",
      backHome: "Volver al inicio",
    },
    finalSubmit: {
      title: "Finalizando su respuesta",
      body: "Bloqueamos la edicion mientras terminamos de transcribir y enviar su respuesta.",
    },
    disclaimer: {
      scrollToContinue: "Desplacese hasta el final del aviso para continuar.",
      agree: "Entiendo",
    },
    progress: {
      timeRemaining: "Tiempo máximo restante",
    },
    inputMode: {
      text: "Entrada por texto",
      voice: "Entrada por voz",
    },
    textAnswer: {
      submitWindowLabel: "Envie esta respuesta en",
      finalDraft: "Borrador final",
      answerInSelectedLanguage: "Responda en su idioma seleccionado",
      placeholder: "Escriba su respuesta aqui...",
      characters: "Caracteres",
      timeExpired: "Se acabo el tiempo - por favor envie su respuesta ahora.",
      submit: "Enviar respuesta",
    },
    page: {
      loading: "Cargando...",
      processingAnswer: "Procesando respuesta",
      backHome: "Volver al inicio",
      understandAnswer: "Entiendo",
      stopRecordingBeforeSwitch: "Detenga la grabacion antes de cambiar de modo.",
      discardVoiceSwitchConfirm: "Tiene audio sin revisar. Desea descartarlo y cambiar de modo?",
    },
    report: {
      completedTitle: "Entrevista completada",
      alreadyCompletedBody: "Esta entrevista ya estaba finalizada. Puede generar su reporte aqui mismo.",
      finishedBody: "Todos los criterios fueron evaluados. Cuando este listo, genere su reporte.",
      generate: "Generar reporte",
      loading: "Cargando reporte",
      generating: "Generando reporte",
      checkAgain: "Verificar de nuevo",
      errorPrefix: "Error:",
      tryAgain: "Intentar de nuevo",
      print: "Imprimir / Guardar como PDF",
      printHint: "En movil: toque Imprimir y luego Guardar como PDF.",
      summaryTitle: "Resumen de retroalimentacion para preparacion",
      questionsMinutes: (questionCount: number, durationMinutes: number) => `${questionCount} preguntas - ${durationMinutes} minutos`,
      clarityTitle: "Areas de claridad",
      developTitle: "Areas para desarrollar mas",
      recommendationTitle: "Recomendacion",
      fullAssessmentTitle: "Evaluacion completa",
      noItems: "Sin elementos para mostrar.",
    },
    voice: {
      submitWindowLabel: "Envie esta respuesta en",
      reviewWarning: "Detengase pronto para dejar tiempo para revisar antes de enviar.",
      playbackAria: "Reproducir audio grabado",
      reviewableTranscript: "Transcripcion revisable",
      transcriptPlaceholder: "Edite la transcripcion aqui...",
      rerecord: "Regrabar",
      discard: "Descartar",
      complete: "Complete",
      timeExpired: "Se acabo el tiempo - por favor envie su respuesta ahora.",
      reviewTranscript: "Revisar transcripcion",
      reviewingTranscript: "Revisando transcripcion...",
      submitAnswer: "Enviar respuesta",
      audioReadyLabel: "Audio listo",
      warningBeforeLimit: (remainingSeconds: number, limitLabel: string) => `Quedan ${remainingSeconds}s para llegar al limite de ${limitLabel}.`,
    },
    microphoneDialog: {
      dismissReconnect: "Cerrar",
      dismissInitial: "Ahora no",
    },
    timeoutDialog: {
      eyebrow: "Tiempo de respuesta terminado",
      title: "Se acabo el tiempo",
      body: "Envie ahora para continuar con la siguiente pregunta.",
      status: "Su respuesta esta lista para enviarse.",
      interruptedTitle: "Esta ventana de respuesta ya termino",
      interruptedBody: "Esta pagina se abrio de nuevo despues de que termino el tiempo de respuesta. Envie ahora para continuar.",
      interruptedStatus: "Su respuesta anterior ya no puede seguir editandose despues de la interrupcion.",
      transcribingTitle: "Transcribiendo su grabacion",
      transcribingBody: "Estamos convirtiendo su grabacion en texto para que pueda enviar esta respuesta.",
      transcribingStatus: "Transcripcion en progreso.",
    },
  },
} as const satisfies Record<Lang, InterviewCopy>;

export function getInterviewMessages(lang: Lang) {
  return INTERVIEW_MESSAGES[lang];
}

export function getAnswerTimerMessage(lang: Lang, answerSecondsLeft: number): string {
  if (answerSecondsLeft <= 30) {
    return lang === "es"
      ? "Quedan 0:30 o menos. Termine y envie su respuesta."
      : "0:30 or less remain. Finish and submit your answer.";
  }
  if (answerSecondsLeft <= 60) {
    return lang === "es"
      ? "Queda 1:00 o menos. Termine y envie su respuesta."
      : "1:00 or less remain. Finish and submit your answer.";
  }
  return lang === "es"
    ? "Use este tiempo para revisar y enviar su respuesta final."
    : "Use this time to review and submit your final answer.";
}

export function getVoiceReviewWarning(lang: Lang): string {
  return getInterviewMessages(lang).voice.reviewWarning;
}

export function getRecorderStatusMessage(
  lang: Lang,
  voiceRecorderState: VoiceRecorderState,
  voiceIsRecordingPaused: boolean,
  voiceIsRecordingActive: boolean,
): string {
  if (voiceRecorderState === "idle") {
    return lang === "es" ? "Pulse Record para empezar." : "Press Record to begin.";
  }
  if (voiceIsRecordingPaused) {
    return lang === "es" ? "Grabacion en pausa." : "Recording paused.";
  }
  if (voiceIsRecordingActive) {
    return lang === "es" ? "Grabando..." : "Recording...";
  }
  if (voiceRecorderState === "transcribing_for_review") {
    return lang === "es" ? "Preparando la transcripcion..." : "Preparing transcript...";
  }
  if (voiceRecorderState === "review_ready") {
    return lang === "es"
      ? "Transcripcion lista. Revisela y envie su respuesta."
      : "Transcript ready. Review it and submit your answer.";
  }
  return lang === "es"
    ? "Audio listo. Revise la transcripcion antes de enviar."
    : "Audio ready. Review the transcript before submitting.";
}

export function getMicSetupCopy(
  lang: Lang,
  hasMicOptIn: boolean,
  micWarmState: MicWarmState,
): { message: string; buttonLabel: string; variant: AlertVariant; busy: boolean } {
  if (micWarmState === "warming") {
    return {
      message: lang === "es"
        ? "Conectando el microfono. Esto puede tardar un momento."
        : "Connecting the microphone. This can take a moment.",
      buttonLabel: lang === "es" ? "Conectando..." : "Connecting...",
      variant: "info",
      busy: true,
    };
  }

  if (micWarmState === "recovering") {
    return {
      message: lang === "es"
        ? "Reconectando el microfono para que la proxima grabacion empiece sin problemas."
        : "Reconnecting the microphone so the next recording can start cleanly.",
      buttonLabel: lang === "es" ? "Reconectando..." : "Reconnecting...",
      variant: "warning",
      busy: true,
    };
  }

  if (micWarmState === "denied") {
    return {
      message: lang === "es"
        ? "No se concedio acceso al microfono. Vuelva a intentarlo cuando quiera usar voz."
        : "Microphone access was not granted. Try again when you want to use voice.",
      buttonLabel: lang === "es" ? "Habilitar microfono" : "Enable microphone",
      variant: "error",
      busy: false,
    };
  }

  if (micWarmState === "error") {
    return {
      message: lang === "es"
        ? "El microfono necesita reconectarse antes de la proxima grabacion."
        : "The microphone needs to reconnect before the next recording.",
      buttonLabel: lang === "es" ? "Reconectar microfono" : "Reconnect microphone",
      variant: "error",
      busy: false,
    };
  }

  return {
    message: hasMicOptIn
      ? (lang === "es"
        ? "El microfono quedara listo mientras continua con la entrevista."
        : "The microphone will stay ready while you continue the interview.")
      : (lang === "es"
        ? "Si piensa responder por voz, habilite el microfono ahora para evitar demora cuando grabe."
        : "If you plan to answer by voice, enable the microphone now to avoid delay when you record."),
    buttonLabel: lang === "es" ? "Habilitar microfono" : "Enable microphone",
    variant: hasMicOptIn ? "info" : "warning",
    busy: false,
  };
}

export function getMicrophoneWarmupDialogCopy(
  lang: Lang,
  mode: MicrophoneWarmupDialogMode,
  uiState: MicrophoneWarmupDialogState,
) {
  if (uiState === "ready_handoff") {
    return {
      eyebrow: lang === "es" ? "Microfono listo" : "Microphone ready",
      title: lang === "es" ? "Su microfono ya esta preparado" : "Your microphone is ready",
      body: lang === "es"
        ? "Terminando la conexion para que la entrevista continue sin interrupciones."
        : "Finishing the connection so the interview can continue without interruption.",
      status: lang === "es" ? "Microfono conectado correctamente." : "Microphone connected successfully.",
      statusVariant: "success" as const,
      buttonLabel: lang === "es" ? "Conectando..." : "Connecting...",
    };
  }

  if (uiState === "warming") {
    return {
      eyebrow: lang === "es" ? "Configurando microfono" : "Preparing microphone",
      title: lang === "es" ? "Preparando el microfono" : "Preparing the microphone",
      body: lang === "es"
        ? "La primera conexion puede tardar un poco. Mantenga esta ventana abierta mientras terminamos."
        : "The first connection can take a moment. Keep this window open while we finish.",
      status: lang === "es" ? "Conectando el microfono. Esto puede tardar un momento." : "Connecting the microphone. This can take a moment.",
      statusVariant: "info" as const,
      buttonLabel: lang === "es" ? "Conectando..." : "Connecting...",
    };
  }

  if (uiState === "recovering") {
    return {
      eyebrow: lang === "es" ? "Reconectando microfono" : "Reconnecting microphone",
      title: lang === "es" ? "Volviendo a preparar el micrófono" : "Preparing the microphone again",
      body: lang === "es"
        ? "La conexion del microfono se interrumpio. Estamos intentando recuperarla antes de continuar."
        : "The microphone connection was interrupted. We are restoring it before continuing.",
      status: lang === "es" ? "Reconectando el micrófono." : "Reconnecting the microphone.",
      statusVariant: "warning" as const,
      buttonLabel: lang === "es" ? "Reconectando..." : "Reconnecting...",
    };
  }

  if (uiState === "denied") {
    return {
      eyebrow: lang === "es" ? "Permiso requerido" : "Permission required",
      title: lang === "es" ? "Necesitamos acceso al microfono" : "We need microphone access",
      body: lang === "es"
        ? "Puede intentarlo otra vez cuando quiera responder por voz."
        : "You can try again whenever you want to answer by voice.",
      status: lang === "es"
        ? "No se concedio permiso al microfono. Puede volver a intentarlo."
        : "Microphone permission was not granted. You can try again.",
      statusVariant: "error" as const,
      buttonLabel: lang === "es" ? "Habilitar microfono" : "Enable microphone",
    };
  }

  if (uiState === "error") {
    return {
      eyebrow: lang === "es" ? "Microfono no disponible" : "Microphone unavailable",
      title: lang === "es" ? "No pudimos preparar el microfono" : "We could not prepare the microphone",
      body: lang === "es"
        ? "Intentelo otra vez. Si el problema continua, puede seguir con texto por ahora."
        : "Try again. If the problem continues, you can keep going with text for now.",
      status: lang === "es"
        ? "El microfono necesita reconectarse antes de la proxima grabacion."
        : "The microphone needs to reconnect before the next recording.",
      statusVariant: "error" as const,
      buttonLabel: lang === "es"
        ? (mode === "reconnect" ? "Reconectar microfono" : "Reintentar microfono")
        : (mode === "reconnect" ? "Reconnect microphone" : "Retry microphone"),
    };
  }

  return {
    eyebrow: lang === "es" ? "Microfono opcional" : "Optional microphone setup",
    title: lang === "es" ? "Prepare el microfono ahora" : "Prepare the microphone now",
    body: lang === "es"
      ? "Si piensa responder por voz, este es el mejor momento para conceder permiso al microfono. Una vez habilitado, el indicador del navegador puede permanecer encendido hasta la etapa del reporte."
      : "If you plan to answer by voice, this is the best time to grant microphone permission. Once enabled, the browser indicator may stay on until the report stage.",
    status: lang === "es"
      ? "Si piensa responder por voz, habilite el microfono ahora para evitar demora cuando grabe."
      : "If you plan to answer by voice, enable the microphone now to avoid delay when you record.",
    statusVariant: "info" as const,
    buttonLabel: lang === "es" ? "Habilitar microfono" : "Enable microphone",
  };
}

export function getReportIntroMessage(lang: Lang, completionSource: CompletionSource): string {
  const copy = getInterviewMessages(lang).report;
  return completionSource === "already_completed"
    ? copy.alreadyCompletedBody
    : copy.finishedBody;
}

export function getReportErrorMessage(lang: Lang, error: ReportErrorState | null): string {
  if (!error) return "";

  switch (error.code) {
    case "generation_failed":
      return lang === "es"
        ? "La generacion del reporte fallo. Intente de nuevo."
        : "Report generation failed. Please try again.";
    case "polling_timed_out":
      return lang === "es"
        ? "La espera del reporte excedio el tiempo limite. Intente de nuevo."
        : "Report polling timed out. Try again.";
    case "polling_paused":
      return lang === "es"
        ? "La verificacion del reporte se detuvo despues de fallas repetidas. Intente de nuevo."
        : "Report polling paused after repeated failures. Try again.";
    case "network":
      return lang === "es"
        ? "Error de conexion al cargar el reporte. Intente de nuevo."
        : "Connection error while loading the report. Please try again.";
    case "queue_failed":
      return lang === "es"
        ? "No se pudo iniciar el reporte. Intente de nuevo."
        : "Could not start report generation. Please try again.";
    case "load_failed":
      return lang === "es"
        ? "No se pudo cargar el reporte. Intente de nuevo."
        : "Could not load the report. Please try again.";
    default:
      return lang === "es"
        ? "No se pudo cargar el reporte. Intente de nuevo."
        : "Could not load the report. Please try again.";
  }
}

export function getVoiceFeedbackMessage(lang: Lang, feedback: VoiceFeedback | null): string {
  if (!feedback) return "";

  switch (feedback.code) {
    case "microphone_permission_denied":
      return lang === "es"
        ? "Permita el acceso al microfono para grabar por voz."
        : "Allow microphone access to record by voice.";
    case "microphone_unavailable":
      return lang === "es"
        ? "No se pudo acceder al microfono."
        : "Unable to access microphone.";
    case "browser_unsupported":
      return lang === "es"
        ? "Este navegador no soporta grabacion de audio."
        : "This browser does not support audio recording.";
    case "secure_context_required":
      return lang === "es"
        ? "La grabacion de voz requiere HTTPS o localhost."
        : "Voice recording requires HTTPS or localhost.";
    case "recording_failed":
      return lang === "es"
        ? "No se pudo completar la grabacion."
        : "Unable to complete recording.";
    case "switch_mode_while_recording":
      return lang === "es"
        ? "Detenga la grabacion antes de cambiar de modo."
        : "Stop recording before switching modes.";
    case "no_audio_detected":
      return lang === "es"
        ? "No se detecto audio. Intente grabar de nuevo."
        : "No audio detected. Please record again.";
    case "audio_playback_failed":
      return lang === "es"
        ? "No se pudo reproducir el audio grabado."
        : "Unable to play the recorded audio.";
    case "transcription_failed":
      return lang === "es"
        ? "La transcripcion de audio fallo. Intente de nuevo."
        : "Audio transcription failed. Please try again.";
    case "session_unauthorized":
      return lang === "es"
        ? "La sesion no esta autorizada para usar voz."
        : "This session is not authorized to use voice.";
    case "voice_api_unavailable":
      return lang === "es"
        ? "El servicio de voz no esta disponible en este momento."
        : "Voice service is not available right now.";
    case "limit_reached":
      return lang === "es"
        ? `Se alcanzo el limite de ${VOICE_MAX_SECONDS / 60} minutos y la grabacion se detuvo.`
        : `The limit of ${VOICE_MAX_SECONDS / 60} minutes was reached and recording stopped.`;
    case "audio_ready":
      return lang === "es"
        ? "Audio listo. Revise la transcripcion antes de enviar."
        : "Audio ready. Review the transcript before submitting.";
    case "preparing_transcript":
      return lang === "es"
        ? "Preparando la transcripcion para revisar..."
        : "Preparing the transcript for review...";
    case "transcript_ready":
      return lang === "es"
        ? "Transcripcion lista. Revisela y envie su respuesta."
        : "Transcript ready. Review it and submit your answer.";
    case "audio_ready_retry_review":
      return lang === "es"
        ? "Audio listo. Puede intentar revisar la transcripcion otra vez."
        : "Audio ready. You can try reviewing the transcript again.";
    default:
      return "";
  }
}

export function getTimeoutDialogCopy(
  lang: Lang,
  options: { isInterrupted: boolean; isTranscribing: boolean },
) {
  const timeout = getInterviewMessages(lang).timeoutDialog;
  const submitLabel = getInterviewMessages(lang).textAnswer.submit;
  const transcribingLabel = getInterviewMessages(lang).voice.reviewingTranscript;

  if (options.isTranscribing) {
    return {
      eyebrow: timeout.eyebrow,
      title: timeout.transcribingTitle,
      body: timeout.transcribingBody,
      status: timeout.transcribingStatus,
      buttonLabel: transcribingLabel,
      statusVariant: "info" as const,
    };
  }

  if (options.isInterrupted) {
    return {
      eyebrow: timeout.eyebrow,
      title: timeout.interruptedTitle,
      body: timeout.interruptedBody,
      status: timeout.interruptedStatus,
      buttonLabel: submitLabel,
      statusVariant: "warning" as const,
    };
  }

  return {
    eyebrow: timeout.eyebrow,
    title: timeout.title,
    body: timeout.body,
    status: timeout.status,
    buttonLabel: submitLabel,
    statusVariant: "warning" as const,
  };
}

export { INTERVIEW_MESSAGES };
