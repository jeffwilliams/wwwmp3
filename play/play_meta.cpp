#include <id3/tag.h>
#include <iconv.h>
#include "play.h"
#include "string.h"

static char* empty_string() {
  char* rc = new char[1];
  rc[0] = '\0';
  return rc;
}

// Caller must delete [] dst after use.
static void field_text(char** dst, ID3_Frame* frame) {
  iconv_t cd = iconv_open("UTF8","UTF16BE");
  const int bufsize = 1000;
  char buf[bufsize];

  if (NULL != frame) {
    ID3_Field* field = frame->GetField(ID3FN_TEXT);
    if (NULL != field) {

      switch(field->GetEncoding()){
      case ID3TE_UTF16:
      case ID3TE_UTF16BE:
        // Convert to UTF-8 before returning the data to Go.
        if ( cd != (iconv_t) -1 ) {
          char* in = (char*) field->GetRawUnicodeText();
          size_t insize = field->Size();

          char* bufptr = buf;
          size_t bufbytes = bufsize;
          size_t rc = 0;

          // Initialize iconv state
          if( iconv(cd, NULL, NULL, &bufptr, &bufbytes) == (size_t) -1 ){
            *dst = empty_string();
            break;
          }

          if ( (rc = iconv(cd, &in, &insize, &bufptr, &bufbytes)) != (size_t) -1 ) {
            *bufptr = '\0';
            *dst = new char[bufsize-bufbytes+1];
            memcpy(*dst, buf, bufsize-bufbytes);
            (*dst)[bufsize-bufbytes] = '\0';
          } else {
            *dst = empty_string();
          }
        } else {
          *dst = empty_string();
        }
        break;
      default:
        // Ascii
        *dst = new char[field->Size()+1];
        (*dst)[0] = '\0';
        size_t len = field->Get(*dst, field->Size()+1);
        (*dst)[field->Size()] = '\0';
      }
    }
  }

  iconv_close(cd);
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
  field_text(&result.tracknum, tag.Find(ID3FID_TRACKNUM));

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
  char buf[1000];

  // Iconv conversion descriptor: UTF-16 -> UTF-8
  iconv_t cd = iconv_open("UTF8","UTF16BE");

  while (NULL != (frame = iter->GetNext()))
  {
    char* val = NULL;
    field_text(&val, frame); 
    printf("Frame: type %d %s %s:\n", frame->GetID(), frame->GetTextID(), frame->GetDescription());
    ID3_Frame::Iterator* fieldIter = frame->CreateIterator();
    ID3_Field* field = NULL;

    while (NULL != (field = fieldIter->GetNext())) {
      printf("  Field: type ");
      ID3_FieldType type = field->GetType();
      switch(type) {
      case ID3FTY_NONE:
        printf("none"); break;
      case ID3FTY_INTEGER:
        printf("int, id %d: %u",
            field->GetID(), 
            (uint32) field->Get());
        break;
      case ID3FTY_BINARY:
        printf("binary"); break;
      case ID3FTY_TEXTSTRING:
        field->Get(buf, 1000);
        printf("text with %d items, encoding ", field->GetNumTextItems());
        switch(field->GetEncoding()){
        case ID3TE_UTF16: 
          printf("UTF-16"); 

          if ( cd != (iconv_t) -1 ) {
            char* in = (char*) field->GetRawUnicodeText();
            size_t insize = field->Size();

            char* bufptr = buf;
            size_t bufsize = 1000;
            size_t rc = 0;

            // Initialize iconv state
            if( iconv(cd, NULL, NULL, &bufptr, &bufsize) == (size_t) -1 ){
              printf("iconv init Failed\n");
            }
            if ( (rc = iconv(cd, &in, &insize, &bufptr, &bufsize)) != (size_t) -1 ) {
              *bufptr = '\0';
              printf(": '%s' (%d chars)\n", buf, rc);
            } else {
              printf("<conversion using iconv failed>");
              perror("iconv");
            }
          }
          break;
        case ID3TE_UTF16BE: 
          printf("UTF-16BE"); 
          printf(": '%s'", buf);
          break;
        case ID3TE_UTF8: 
          printf("UTF-8");
          printf(": '%s'", buf);
          break;
        case ID3TE_NONE: 
          printf("none");
          printf(": '%s'", buf);
          break;
        case ID3TE_ISO8859_1: 
          printf("ISO-8859-1/ASCII");
          printf(": '%s'", buf);
          break;
        }
        break;
      default:
        printf("unknown");
      }

      printf("\n");
    }
    delete fieldIter;
    delete [] val;
  }
  delete iter;

  iconv_close(cd);
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
