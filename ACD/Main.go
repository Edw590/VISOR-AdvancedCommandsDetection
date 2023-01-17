/*
 * Copyright 2021 DADi590
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package ACD

import (
	"fmt"
	"strconv"
	"strings"

	"AdvancedCommandsDetection/Tcf"
)

const ERR_CMD_DETECT string = MOD_RET_ERR_PREFIX + " - "

/*
Main is the function to call to request a detection of commands in a given sentence of words.

-----------------------------------------------------------

> Params:
  - sentence_str – a sentence of words, for example coming directly from speech recognition
  - remove_repet_cmds – true to remove repeated adjacent commands and leave only the last one, false to not remove them.
    Note that repeated commands means any commands with the same ID, whether or not they have the same sub-output. Example:
    "1.01, 1.02, 2.01, 1.01" --> "1.02, 2.01, 1.01".
  - invalidate_detec_words – same as in sentenceCmdsDetector()

> Returns:

– a list of the detected commands in the form

	[CMD1][separator][CMD2][separator][CMD3]

with CMDS_SEPARATOR as the separator; if the function detected no commands, an empty string; if any error occurred, a
string beginning with ERR_CMD_DETECT, followed either by GlobalUtils_APU.APU_ERR_PREFIX and its requirements, or a Go
error
*/
func Main(sentence_str string, remove_repet_cmds bool, invalidate_detec_words bool) string {
	var ret_var string = ""

	Tcf.Tcf{
		Try: func() {
			ret_var = MainInternal(sentence_str, remove_repet_cmds, invalidate_detec_words)
		},
		Catch: func(e Tcf.Exception) {
			ret_var = ERR_CMD_DETECT + fmt.Sprint(e)
		},
	}.Do()

	return ret_var
}

const CMDS_SEPARATOR string = ", "

/*
MainInternal is the actual function that will do what's written on Main() - continue reading there.

There is just one exception, which is this one doesn't return any error code if anything goes wrong - it will panic
instead (no protection here), so always call the other one in production code.

Note: if you find this function exported, know it's just for testing from the main package. Do NOT use it in production.
*/
func MainInternal(sentence_str string, remove_repet_cmds bool, invalidate_detec_words bool) string {
	if "" == strings.TrimSpace(sentence_str) {
		// If the string is empty on visible characters (space counts as invisible here...), return now, because the
		// code ahead may not work with strings like that (and some of it does not - panic --> reason I'm returning
		// here).

		return ""
	}

	sentence_str = sentenceCorrection(sentence_str, nil, true)

	var sentence []string = strings.Split(sentence_str, " ")

	// Prepare the sentence for the NLP analysis
	sentence_str = sentenceNLPPreparation(sentence_str, &sentence, true)
	// Analyze the sentence with NLP help and, for example, replace all the "it"s on the sentence with their meaning
	sentence_str = nlpAnalyzer(&sentence, sentence_str)
	// "Unprepare" what was prepared on the sentence for the NLP analysis
	/*sentence_str = */
	sentenceNLPPreparation(sentence_str, &sentence, false) //--> uncomment the beginning if sentence_str is needed

	sentenceCorrection("", &sentence, false)

	//log.Println(sentence)

	// Get all the commands present on the sentence.
	var sentence_cmds []float32 = sentenceCmdsDetector(sentence, invalidate_detec_words)

	// Filter the sentence of special commands (like "don't"/"do not") and do the necessary for each special command.
	taskFilter(&sentence_cmds)

	var ret_var string = ""
	for _, command := range sentence_cmds {
		ret_var += fmt.Sprint(command) + CMDS_SEPARATOR
	}

	//log.Println("::::::::::::::::::::::::::::::::::")
	//log.Println(ret_var)

	if "" != ret_var {
		ret_var = ret_var[:len(ret_var)-len(CMDS_SEPARATOR)]
	}

	// Remove consecutively repeated commands
	// Let's see if the verification function can handle it without this...
	// EDIT: it could very well (improved a lot since then), but it's needed again, at least sometimes. So I'm putting
	// it optional.
	if remove_repet_cmds {
		ret_var = removeRepeatedCmds(ret_var)
	}

	//log.Println(ret_var)
	//log.Println("::::::::::::::::::::::::::::::::::")

	return ret_var
}

/*
removeRepeatedCmds removes immediately repeated commands from the commands verification ([1, 3, 3, 4, 3, 4] will become
[1, 3, 4, 3, 4], for example).

This function attempts to kind of fix the problem of wrongly detected repeated commands.

For example, with the command (punctuation added for better understanding - remove it to test):
"turn on wifi and get the airplane mode on. no, don't turn the wifi on. turn off airplane mode and turn the wifi on.",
the command detection returns:

	"3234_wifi(),on \\// 3234_wifi(),on \\// 3234_wifi(),on \\// 3234_wifi(),on \\// 3234_wifi(),on \\//
	3234_airplane_mode(),on \\// 3234_airplane_mode(),on \\// 3234_wifi(),on \\// 3234_wifi(),on \\// 3234_wifi(),on
	\\// ".

Awfully wrong. This function improves that to "3234_wifi(),on \\// 3234_airplane_mode(),on \\// 3234_wifi(),on".
The first command is still wrong, the but idea here is to delete all the repeated elements (which improved MUCH in this
case).

Though, this also poses the problem of deleting purposefully repeated commands... Will be used until the
wordsVerificationFunction() can do the job better. In that case might be better (for now) to say the repeated commands in
another function call.

(As a curiosity, the overall Main() function can now know what to do in the example above, without needing to execute
this function at all!!! A thanks to this might be due to the new parameter on the wordsVerificationFunction() that
ignores possibly repeated commands!)
*/
func removeRepeatedCmds(detected_cmds_str_param string) string {
	var detected_cmds_list []string = strings.Split(detected_cmds_str_param, CMDS_SEPARATOR)

	const MARK_TERMINATION_STR string = "3234_TERM"

	var detected_cmds_list_len_1 int = len(detected_cmds_list) - 1
	for i, j := range detected_cmds_list {
		if i != detected_cmds_list_len_1 {
			var next_j string = detected_cmds_list[i+1]
			var next_j_index int = strings.Index(next_j, ".")
			var j_index int = strings.Index(j, ".")
			//log.Println("-------")
			//log.Println(prev_j)
			//log.Println(prev_j[:prev_j_index)
			//log.Println(j)
			//log.Println(j[:j_index)
			if (-1 != next_j_index) && (-1 != j_index) && (j[:j_index] == next_j[:next_j_index]) {
				detected_cmds_list[i] = MARK_TERMINATION_STR
			}
		}
	}

	var detected_cmds_str string = ""
	for _, command := range detected_cmds_list {
		if command != MARK_TERMINATION_STR {
			detected_cmds_str += command + CMDS_SEPARATOR
		}
	}

	if strings.HasSuffix(detected_cmds_str, CMDS_SEPARATOR) {
		detected_cmds_str = detected_cmds_str[:len(detected_cmds_str)-len(CMDS_SEPARATOR)]
	}

	return detected_cmds_str
}

const ANY_MAIN_WORD string = ";4;"

// ATTENTION - none of these constants below can collide with the WARN_-started constants on CmdsInfo!!!
// const spec_cmd_dont_instead_CONST float32 = -1.1
// const spec_cmd_stop_CONST float32 = -2
// const spec_cmd_forget_CONST float32 = -3
const spec_cmd_dont_CONST float32 = -1

const invalidate_word_CONST string = ";5;"

/*
sentenceCmdsDetector detects which of the cmds_GL commands are present in a sentence of words.

-----------------------------------------------------------

> Params:
  - sentence – a 1D slice of words on which the verification will be executed (basically it's sentence_str required by
    Main() split by spaces in a 1D slice).
  - invalidate_detec_words – true to invalidate words used on detections so that they're not used on further detections
    (useful to prevent wrong detections), false otherwise. Example of a problematic sentence: "fast reboot the phone", with
    "fast" and "reboot" being both command main words - 2 command detections will be triggered and phone (fast reboot and
    reboot normally) --> with this set to true, not anymore, because each word used on a successful detection will be
    replaced by invalidate_word_CONST and hence will not be used again.

> Returns:

– a slice on which each index is a command found in the 'sentence' in the order provided by the 'sentence'. The command
is a float in which the integer part is the index of the command on cmds_GL and the decimal part is the index+1 of the
detected condition of the command, with each condition incrementing by 0.01. For example, for

	{ // 14
		{{{-1}, {"device", "phone"}}, {{-1}, {"safe"}}, {-1: {"mode"}}},
		{{{-1}, {"device", "phone"}}, {{-1}, {"recovery"}}},
		{{{-1}, {"device", "phone"}}},
	},

and the sentence "reboot the device to recovery", the output will be 14.02 (command ID 14, 2nd condition).
*/
func sentenceCmdsDetector(sentence []string, invalidate_detec_words bool) []float32 {
	var detected_cmds []float32 = nil

	for sentence_counter, sentence_word := range sentence {

		if "don't" == sentence_word {
			detected_cmds = append(detected_cmds, spec_cmd_dont_CONST)
		} else if WHATS_IT == sentence_word {
			float, _ := strconv.ParseFloat(WARN_WHATS_IT, 32)
			detected_cmds = append(detected_cmds, float32(float))
		} else {
			for i := range cmds_GL {
				for _, main_word := range cmds_GL[i].main_words {
					if main_word == sentence_word {
						// Uncomment for testing
						//if 14 != cmds_GL[i].cmd_id {
						//	continue
						//}

						//log.Println("==============")
						//log.Println(sentence_word)
						//log.Println(i)

						var results_WordsVerificationDADi [][][]interface{} = wordsVerificationFunction(sentence,
							sentence_counter, cmds_GL[i])

						//log.Println("-----------")
						//log.Println(results_WordsVerificationDADi)

						if len(results_WordsVerificationDADi) > 0 {
							var final_condition int = -1
							// Must be the biggest condition because, for example "reboot phone" and "reboot phone into
							// recovery", and the sentence is "reboot phone into recovery". Both are successful
							// detections (all words are found in both variations). But only the 2nd (the *biggest*) is
							// correct, because more words were found, and more words has higher priority than fewer
							// words.
							var biggest_len int = -1

							//log.Println(success_detects)
							for ii, jj := range results_WordsVerificationDADi {
								var all_true bool = true
								for _, jjj := range jj {
									all_true = all_true && jjj[0].(bool)
								}
								if all_true {
									var main_words_ret_conds [][]string = cmds_GL[i].main_words_ret_conds
									var arr_id int = 0
									if ii >= len(main_words_ret_conds) {
										// In case there are not enough return conditions, use the last one present.
										arr_id = len(main_words_ret_conds) - 1
									} else {
										arr_id = ii
									}
									if (1 == len(main_words_ret_conds[arr_id])) &&
										(ANY_MAIN_WORD == main_words_ret_conds[arr_id][0]) {
										if len(jj) > biggest_len {
											final_condition = ii
											biggest_len = len(jj)
										}
									} else {
										for _, word := range main_words_ret_conds[arr_id] {
											if word == sentence_word {
												if len(jj) > biggest_len {
													final_condition = ii
													biggest_len = len(jj)

													break
												}
											}
										}
									}
								}
							}

							if final_condition != -1 {
								detected_cmds = append(detected_cmds,
									float32(final_condition+1)/100+float32(cmds_GL[i].cmd_id))
								// results_WordsVerificationDADi + 1 because 0.00 must not happen
								// / 100 to go from 0+1 = 1 to 0.01
								// + cmd_index because what returns from the function is the return command ID for that
								// specific command - not a global one --> this makes it global (always different)

								if invalidate_detec_words {
									sentence[sentence_counter] = invalidate_word_CONST
									for _, j := range results_WordsVerificationDADi[final_condition] {
										var index int = j[1].(int)
										if index >= 0 {
											sentence[index] = invalidate_word_CONST
										}
									}
								}

								//log.Println("-----------")
								//log.Println(results_WordsVerificationDADi[final_condition])

								//log.Println(sentence)
							}
						}
					}
				}
			}
		}
	}

	return detected_cmds
}

/*
taskFilter filters a sentence of commands depending on special commands present on it.

For example, "turn on the lights and play some music. no, don't turn on the lights" --> the special command here is
"don't", and in this case the function will only leave on the slice the music command.

-----------------------------------------------------------

> Params:
  - sentence_cmds – same as in sentenceCmdsDetector()

> Returns:
  - nothing
*/
func taskFilter(sentence_cmds *[]float32) {
	// For testing
	//*sentence_filtered = [][]string{{"test"}, {"test"}, {"test 234 lkj"}, {"test"}, {"test"}, {"test"}, {"test"},
	//	{"test"}, {"test"}, {"test"}, {"test"}, {"test"}, {"test"}, {"test"}, {"test"}, }
	//*sentence_cmds = []float32{24, -1, 26, 25, -1, -1, -1, 25, 24}

	//log.Println("==============================================")
	//log.Println("*sentence_cmds -->", *sentence_cmds)

	// RESTRICTED VALUE ON THE sentence_cmds SLICE - Used to mark elements for deletion on the slice. This way, they're
	// deleted only in the end and on the main loop it doesn't get confusing about which elements have been deleted
	// already.
	const MARK_TERMINATION_FLOAT32 float32 = 0

	for counter, number := range *sentence_cmds {
		if spec_cmd_dont_CONST == number {

			var delete_number_before_dont bool = false

			// Delete the "don't"
			(*sentence_cmds)[counter] = MARK_TERMINATION_FLOAT32

			//log.Println("1 -", *sentence_cmds)
			if counter != len(*sentence_cmds)-1 {
				// If the next index is within the maximum index (which means, if the next number exists)...

				var next_number float32 = (*sentence_cmds)[counter+1]
				if next_number > 0 { // Means if it's a normal command. If it is, assume the below case.
					// Case: "do [1] and do [2]. no don't do [1]" - delete this, don't, and this. Also, if by any reason
					// there are more copies of [1], delete them also - if they're before the next element only.

					var number_mentioned bool = false
					var pos_next_number []int = nil
					for counter1, number1 := range *sentence_cmds {
						if number1 == next_number {
							pos_next_number = append(pos_next_number, counter1)
							number_mentioned = true
						}
						if counter1 == counter {
							// Stop when it gets to before the next element
							break
						}
					}
					if number_mentioned {
						// If the number was mentioned before (like [24, 25, 24, -1, 24]), delete all copies and the -1.
						(*sentence_cmds)[counter+1] = MARK_TERMINATION_FLOAT32

						//log.Println("2 -", *sentence_cmds)

						for _, index_element := range pos_next_number {
							(*sentence_cmds)[index_element] = MARK_TERMINATION_FLOAT32
						}
						//log.Println("3 -", *sentence_cmds)
					} else {
						// Else, delete only the element before the current "don't" (if there exists one).
						// Example: [24, -1, 26, 25, -1, 25, 24] will become [26, 24], because, "do 24, no don't do
						// it. do 26 and do 25. no don't do 25. do 24."
						delete_number_before_dont = true
					}
				}
				// Else, if it's not a positive number, assume the below case.
				// Case: "do this, no don't do it, don't do it. do that". Delete only the current "don't" as was done
				// above and keep doing it (the loop will automatically) until there's only one, which will be the one
				// used to decide what to delete (done above).
			} else {
				// If there's no more elements, there can be previous ones. So delete the previous number to the "don't".
				// Which would be a "do [1]. no, never mind, don't do it".
				delete_number_before_dont = true
			}

			if delete_number_before_dont {
				// Do it only if there's a normal command before. If it's for example WARN_WHATS_IT, don't delete it.
				if counter-1 >= 0 && (*sentence_cmds)[counter-1] > 0 {
					(*sentence_cmds)[counter-1] = MARK_TERMINATION_FLOAT32
					//log.Println("4 -", *sentence_cmds)
				}
			}
		}
	}

	//log.Println("5 -", *sentence_cmds)

	// Delete all elements marked for deletion
	for counter := 0; counter < len(*sentence_cmds); {
		// Don't forget (again) --> the length must checked every time on the loop because it is changed on it
		if MARK_TERMINATION_FLOAT32 == (*sentence_cmds)[counter] {
			delElemInSlice(sentence_cmds, counter)
		} else {
			counter++
		}
	}

	//log.Println("*sentence_cmds -->", *sentence_cmds)
	//log.Println("==============================================")
}