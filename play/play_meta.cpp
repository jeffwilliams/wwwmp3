#include <id3/tag.h>
#include "play.h"
#include "string.h"

static void field_text(char** dst, ID3_Frame* frame) {
  if (NULL != frame) {
    ID3_Field* field = frame->GetField(ID3FN_TEXT);
    if (NULL != field) {
      *dst = new char[field->Size()+1];
      (*dst)[0] = '\0';
      size_t len = field->Get(*dst, field->Size()+1);
      (*dst)[field->Size()] = '\0';
    }
  }
}

static void set_title_from_filename(char** dst, char* filename){
  int flen = strlen(filename);

  *dst = new char[flen+1];

  int i;
  for(i = flen-1; i >= 0 && filename[i] != '/'; i--){
    //  
  }

  if(filename[i] == '/') 
    i++;

  int j;
  for(j = 0; filename[i] != '\0' && filename[i] != '.'; i++,j++){
    (*dst)[j] = filename[i];
  }
  (*dst)[j] = '\0';
}

extern "C"
play_metadata_t play_meta(char* filename){
  ID3_Tag tag(filename);
  play_metadata_t result;
  memset(&result, 0, sizeof(play_metadata_t));

  field_text(&result.title, tag.Find(ID3FID_TITLE));
  field_text(&result.album, tag.Find(ID3FID_ALBUM));
  field_text(&result.artist, tag.Find(ID3FID_LEADARTIST));

  // If title is not set, set it to the filename (without extension)
  if(!result.title || strlen(result.title) == 0) {
    set_title_from_filename(&result.title, filename);
  }

  return result;
}

extern "C"
void play_debug_meta(char* filename){
  ID3_Tag tag(filename);

  ID3_Tag::Iterator* iter = tag.CreateIterator();
  ID3_Frame* frame = NULL;
  while (NULL != (frame = iter->GetNext()))
  {
    printf("Frame: type %d %s %s\n", frame->GetID(), frame->GetTextID(), frame->GetDescription());
  }
  delete iter;
}              

extern "C"
void play_delete_meta(play_metadata_t meta) {
  if ( meta.title )
    delete [] meta.title;
  if ( meta.artist )
    delete [] meta.artist;
  if ( meta.album )
    delete [] meta.album;
}
