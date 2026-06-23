import type { MessageItem } from "@nexusim/protocol";

export function compareMessagesForDisplay(left: MessageItem, right: MessageItem): number {
  const leftSeq = displaySeq(left);
  const rightSeq = displaySeq(right);
  if (leftSeq !== rightSeq) {
    return leftSeq - rightSeq;
  }
  const createdAtDelta = (left.createdAtMs ?? 0) - (right.createdAtMs ?? 0);
  if (createdAtDelta !== 0) {
    return createdAtDelta;
  }
  return displayStableID(left).localeCompare(displayStableID(right));
}

function displaySeq(message: MessageItem): number {
  if (message.conversationSeq > 0) {
    return message.conversationSeq;
  }
  return Number.MAX_SAFE_INTEGER;
}

function displayStableID(message: MessageItem): string {
  return message.clientMessageID ?? message.messageID;
}
