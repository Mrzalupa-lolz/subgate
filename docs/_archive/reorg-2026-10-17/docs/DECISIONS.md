# Ключевые решения

Приоритет: ответы Q1–Q10 в QUESTIONS.md → PROMPT.md → прочие документы. Полная история: архив `~/SubGate-docs-archive-2026-10-14/DECISIONS.md`.

1. **Q1/F21:** семь анализаторов только сигнализируют; автосанкции запрещены.
2. **Q1/F27:** текстовый Xray-редактор с подсветкой/автодополнением.
3. **Q2:** Telegram разрешён; email/S3/самообновление исключены; IP/ASN-базы разрешены.
4. **Q3:** только Part B; все 37 строк Part A — OUT_OF_SCOPE.
5. **Q5:** Remnawave 3.4.5, Xray 26.7.28, Happ/Incy клиенты.
6. **Q6:** одна owner учётка password-only, без TOTP/RBAC; журнал 7 дней hot + 90 дней compressed, 50% диска.
7. **Q7:** Linux, Go, SQLite/HTTP JSON/vanilla JS; стек менять только по измеренной причине.
8. **Q9:** готовность v1 — все ID в scope DONE с evidence.
9. **Q10:** единый план PROJECT_PLAN.md; исходные два файла требований не менять.
10. **Read-only gate:** panel client по умолчанию отклоняет non-GET до сериализации; write=true только явно.

Источник Q1–Q10: [QUESTIONS.md](registry/QUESTIONS.md).  
Детали R01–R15: архив или PROJECT_PLAN roadmap.
