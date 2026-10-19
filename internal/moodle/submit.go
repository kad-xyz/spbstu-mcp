package moodle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

// UploadDraft uploads local files into one new draft area and returns its item id, which
// submissions and forum posts take to attach the files.
func (c *Client) UploadDraft(ctx context.Context, paths []string) (int, error) {
	if len(paths) == 0 {
		return 0, errors.New("no files to upload")
	}
	token, err := c.session(ctx)
	if err != nil {
		return 0, err
	}
	itemID := 0
	for _, p := range paths {
		id, err := upload(ctx, token, p, itemID)
		if err != nil {
			return 0, fmt.Errorf("upload %s: %w", filepath.Base(p), err)
		}
		itemID = id
	}
	return itemID, nil
}

func upload(ctx context.Context, token, path string, itemID int) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("token", token)
	_ = w.WriteField("filearea", "draft")
	_ = w.WriteField("itemid", strconv.Itoa(itemID))
	part, err := w.CreateFormFile("file_1", filepath.Base(path))
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+"/webservice/upload.php", &buf)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var files []struct {
		ItemID int `json:"itemid"`
	}
	if json.Unmarshal(body, &files) != nil || len(files) == 0 {
		var merr Error
		if json.Unmarshal(body, &merr) == nil && merr.Message != "" {
			return 0, &merr
		}
		return 0, fmt.Errorf("unexpected upload response (HTTP %d)", resp.StatusCode)
	}
	return files[0].ItemID, nil
}

// warnings is the warning list most write functions return.
type warnings []struct {
	Message string `json:"message"`
}

func (w warnings) err() error {
	if len(w) == 0 {
		return nil
	}
	return errors.New(w[0].Message)
}

// SaveSubmission replaces the user's submission for an assignment with these files (and,
// when text is set, online text), then optionally submits it for grading.
func (c *Client) SaveSubmission(ctx context.Context, assignID int, paths []string, text string, submit bool) error {
	params := url.Values{"assignmentid": {strconv.Itoa(assignID)}}
	if len(paths) > 0 {
		itemID, err := c.UploadDraft(ctx, paths)
		if err != nil {
			return err
		}
		params.Set("plugindata[files_filemanager]", strconv.Itoa(itemID))
	}
	if text != "" {
		params.Set("plugindata[onlinetext_editor][text]", text)
		params.Set("plugindata[onlinetext_editor][format]", "2") // plain text
		params.Set("plugindata[onlinetext_editor][itemid]", "0")
	}
	if len(paths) == 0 && text == "" {
		return errors.New("nothing to submit: give files or text")
	}
	var w warnings
	if err := c.Call(ctx, "mod_assign_save_submission", params, &w); err != nil {
		return err
	}
	if err := w.err(); err != nil {
		return err
	}
	if !submit {
		return nil
	}
	params = url.Values{"assignmentid": {strconv.Itoa(assignID)}, "acceptsubmissionstatement": {"1"}}
	if err := c.Call(ctx, "mod_assign_submit_for_grading", params, &w); err != nil {
		return err
	}
	return w.err()
}

// AddDiscussion starts a forum discussion with optional attachments and returns its id.
func (c *Client) AddDiscussion(ctx context.Context, forumID int, subject, message string, paths []string) (int, error) {
	params := url.Values{
		"forumid": {strconv.Itoa(forumID)},
		"subject": {subject},
		"message": {message},
	}
	if len(paths) > 0 {
		itemID, err := c.UploadDraft(ctx, paths)
		if err != nil {
			return 0, err
		}
		params.Set("options[0][name]", "attachmentsid")
		params.Set("options[0][value]", strconv.Itoa(itemID))
	}
	var out struct {
		DiscussionID int      `json:"discussionid"`
		Warnings     warnings `json:"warnings"`
	}
	if err := c.Call(ctx, "mod_forum_add_discussion", params, &out); err != nil {
		return 0, err
	}
	return out.DiscussionID, out.Warnings.err()
}

// Reply posts a reply to a forum post with optional attachments.
func (c *Client) Reply(ctx context.Context, postID int, subject, message string, paths []string) error {
	params := url.Values{
		"postid":  {strconv.Itoa(postID)},
		"subject": {subject},
		"message": {message},
	}
	if len(paths) > 0 {
		itemID, err := c.UploadDraft(ctx, paths)
		if err != nil {
			return err
		}
		params.Set("options[0][name]", "attachmentsid")
		params.Set("options[0][value]", strconv.Itoa(itemID))
	}
	var out struct {
		Warnings warnings `json:"warnings"`
	}
	if err := c.Call(ctx, "mod_forum_add_discussion_post", params, &out); err != nil {
		return err
	}
	return out.Warnings.err()
}
