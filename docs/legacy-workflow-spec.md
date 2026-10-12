> Historical reference copied into GroceriesBot on 2026-10-12. Read `design.md` for the current SQLite design.

Grocery Management Assistant — n8n Flow Spec

Overview

This n8n workflow implements a Telegram-based Grocery Management Assistant that helps a user manage a grocery list stored in Google Sheets. It uses an AI agent to interpret incoming messages, ask follow-up questions when needed, append grocery items, and optionally clear the list with confirmation.

Purpose

The workflow is designed to:

Receive grocery-related messages from Telegram

Interpret user intent using an AI agent

Ask clarifying questions when item details are incomplete

Store grocery items in a Google Sheet

Suggest likely grocery variants based on previous entries

Maintain short conversational context per Telegram chat


Data Model

The Google Sheet Groceries uses these fields:

Name

Category

Quantity

Added DateTime

Purchase DateTime


Rules

Quantity defaults to 1 unless explicitly stated

Quantity only applies to the last mentioned item, then resets to 1

Purchase DateTime must not be set when adding an item

New entries represent pending purchase, not purchased items


Workflow Structure

1. Trigger

Incoming Telegram Message

Node: Incoming Telegram Message
Type: telegramTrigger

Listens for Telegram message updates for:

Chat ID: -5138495076

User ID: 269984362


When a message is received, it starts the workflow.


---

2. Memory

Simple Memory

Node: Simple Memory
Type: memoryBufferWindow

Provides per-chat conversational memory.

Session key: {{ $json.message.chat.id }}

Context window length: 10


This allows the AI agent to keep short-term context across messages in the same Telegram chat.


---

3. Language Model

Claude 4.5 Sonnet

Node: Claude 4.5 Sonnet
Type: lmChatOpenRouter

Provides the LLM used by the AI Agent.

Model: anthropic/claude-sonnet-4.5



---

4. Core Agent

AI Agent

Node: AI Agent
Type: @n8n/n8n-nodes-langchain.agent

This is the main orchestration layer. It receives the Telegram message and determines whether to:

ask the user for clarification

append a grocery item to the sheet

clear the grocery list after confirmation

send a status or completion message


Agent Input

The agent prompt is built as:

Reply to: {{ $json.message.reply_to_message.text }}

{{ $json.message.text }}

Agent System Behavior

The system message defines the assistant as a Grocery Management Assistant with the following key rules:

Help users catalog, organize, and manage groceries

Current time is injected dynamically in Europe/Amsterdam

The sheet consists of name, category, quantity, and purchase date

If the user provides only a generic item such as "milk", ask what kind

Use previous grocery data to suggest likely variants

Add the row when the item is clear enough

Infer category automatically

Do not set purchase date

Quantity defaults to 1 unless clarification is needed

Always communicate with the user using a Telegram tool

Do not reply directly outside tool usage

Exit quietly after tool actions


Agent Settings

promptType: define

maxIterations: 10

returnIntermediateSteps: true

enableStreaming: true



---

5. Google Sheets Tools

Get All Groceries

Node: Get All Groceries
Type: googleSheetsTool
Operation: Read rows

Reads all rows from:

Document ID: 1Kyt9Kp0ShshLwIbbffZ1ul7VkjbE3uVR4TLa0dBLsv0

Sheet: Groceries


Purpose

Used by the AI agent to inspect previous grocery entries and infer likely item variants or patterns.


---

Add Single Item

Node: Add single item
Type: googleSheetsTool
Operation: Append row

Appends a new grocery item to the Groceries sheet.

Mapped Columns

Name

Category

Quantity

Added DateTime


Purchase DateTime exists in schema but is intentionally not set.

Behavior

Uses append mode

Represents a pending grocery item

Matching column is Name

Type conversion is disabled


Purpose

Used when the AI agent has enough information to store a grocery item.


---

Clear Everything

Node: Clear Everything
Type: googleSheetsTool
Operation: Clear sheet

Clears all grocery rows in the Groceries sheet while preserving the header row.

Rule

The tool description explicitly requires confirmation before execution.

Purpose

Used only when the user wants to delete the full grocery list and has confirmed the action.


---

6. Telegram Output Tools

Send a Text Message in Telegram

Node: Send a text message in Telegram
Type: telegramTool

Used to ask the user for additional detail.

Characteristics

Sends message to current chat: {{ $json.message.chat.id }}

Uses reply keyboard

Supports up to 4 buttons

One-time keyboard enabled

No attribution appended


Intended Use

Ask follow-up questions

Offer suggested variants

Prompt for confirmation

Collect missing item details



---

Send a Text Message in Telegram1

Node: Send a text message in Telegram1
Type: telegramTool

Used to send normal plain-text messages.

Characteristics

Sends message to current chat: {{ $json.message.chat.id }}

No reply keyboard

No attribution appended


Intended Use

Final confirmation messages

Status messages

Normal follow-up messages without buttons



---

Execution Flow

Primary Flow

1. Telegram message is received


2. Message is passed to the AI Agent


3. AI Agent uses:

memory context

language model

Google Sheets tools

Telegram tools



4. Agent determines user intent


5. Agent either:

asks for clarification

appends item to sheet

clears sheet after confirmation

sends an informational message



6. Workflow ends without direct output from the agent itself




---

Decision Logic

Add Grocery Item

When the user sends a recognizable grocery item:

Agent checks whether the item is sufficiently specific

If needed, it uses previous grocery history to suggest likely variants

It infers:

Name

Category

Quantity

Added DateTime


It appends the row to Google Sheets

It sends a Telegram confirmation or status message


Example

Input: milk

Expected behavior:

Agent asks what kind of milk

May suggest previously used variants

Once clarified, adds item to sheet



---

Clarification Required

The agent should ask follow-up questions when:

item is too generic, e.g. milk

quantity is ambiguous, e.g. eggs

user intent is incomplete


This is done using the Telegram keyboard tool when buttons are useful, otherwise normal text.


---

Clear Grocery List

When the user wants to remove all groceries:

Agent must ask for confirmation first

Only after confirmation may it call Clear Everything



---

Tooling Relationships

AI Agent Dependencies

The AI Agent is connected to:

Claude 4.5 Sonnet as language model

Simple Memory as conversation memory

Get All Groceries as retrieval tool

Add single item as append tool

Clear Everything as destructive tool

Send a text message in Telegram as interactive follow-up tool

Send a text message in Telegram1 as plain response tool


Trigger Dependency

Incoming Telegram Message feeds directly into AI Agent



---

Functional Constraints

The agent must always use a Telegram tool to communicate with the user

The agent must not directly output a reply outside tool usage

The agent should always exit quietly after tool execution

Purchase DateTime must not be set during item creation

Quantity resets to 1 after applying to the last item

Previous groceries should be consulted to improve suggestions



---

Summary

This workflow is a conversational grocery-list manager for Telegram backed by Google Sheets. It uses an AI agent with short-term memory to interpret grocery requests, resolve ambiguity, store pending grocery items, and manage the list safely through confirmation-driven actions.