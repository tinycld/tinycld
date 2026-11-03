---
title: Web forms
summary: Put a contact or job form on any website and collect the submissions
tags: [forms, "web forms", embed, website, spam]
order: 46
---

A web form is a form that people fill on a website, without an account. You build it in **Settings → Forms → Web form**, then publish it and paste its embed code into the website.

## Build the form

1. Go to **Settings → Forms** and pick **Web form**.
2. Select **New form** and give it a name, for example "Job application".
3. Add fields. A web form can use text, long text, number, date, toggle, choice list, checklist, file, table and calculated fields. Person, rich text and markdown fields need a signed-in user, so a web form does not offer them.
4. For a file field, set **Accepted types** (for example `application/pdf, .docx`) and **Largest file (MB)**.
5. Select **Save**.

## Publish it

1. Open the **Publish** tab.
2. Turn on **Accept submissions**.
3. In **Websites that may show this form**, type each website's address with its scheme, for example `https://example.com`. Separate them with spaces. When the list is empty, any website can show the form.
4. Write the **Message after sending**.
5. Select **Save**, then **Copy embed code**.
6. Paste the code into the website's HTML where the form must show.

The code loads the form from `https://{{server-host}}`. The form sizes itself to its content.

## Spam protection

Every web form has a hidden trap field and a minimum fill time, and limits how often one address can send. For more protection, turn on **Ask the browser to prove it is not a bot**. The visitor's browser then does about one second of work before it sends. Visitors do not see a puzzle. This needs the server on https: browsers do the work only on a secure connection.

## When the website must know a form was sent

The embed code sends a `tinycld:form-submitted` event on the iframe. A web developer can listen for it to show a thank-you page or record a conversion:

    iframe.addEventListener('tinycld:form-submitted', event => { /* event.detail.form */ })

## Turn a form off or change its link

- Turn off **Accept submissions** to stop new submissions. The website then shows "This form is not available."
- **Regenerate link** makes a new embed code. The old code stops working, so paste the new code on each website.

## Send submissions somewhere

A destination is an organization rule that runs for every submission of this form.

1. Open the **Publish** tab and select **Add destination**.
2. The rule starts with **A web form is sent** and this form. Add actions, for example **Send me a notification**.
3. In a title, body or message, insert answers from the form with the field menu. They show as `{{fields.<field>}}`, for example `New lead: {{fields.name}}`. A file field gives its file names.
4. Select **Save**.

To send an email to the address a visitor typed, turn on **Ask the browser to prove it is not a bot** first. Without it, a bot could use your server to send mail to any address.

If a destination fails, admins get one notification for each destination each day.

## See submissions

The **Submissions** tab lists the newest submissions first. Each row shows how many destinations received it, for example "1 of 2 delivered". Open a submission to see its answers, download its files, and select **Retry** for a destination that failed. **Delete** removes the submission and its files; cards and rows a destination already made stay.

## How long submissions are kept

**Keep submissions for (days)** on the Publish tab sets when old submissions and their files are deleted. The default is 90 days. A daily job deletes them.
